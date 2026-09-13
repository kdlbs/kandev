import { expect, type Page } from "@playwright/test";
import type { BackendContext } from "../../fixtures/backend";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForSessionDone } from "../../helpers/session";

export const RETAINED_WORKSPACE_FILE = "walkthrough_base.txt";
export const RETAINED_WORKSPACE_CONTENT = "WALKTHROUGH_UNCHANGED";

type GatewayFrame = {
  id?: string;
  type?: string;
  action?: string;
  payload?: Record<string, unknown>;
};

type SocketMessage = string | Buffer;

function parseGatewayFrame(value: string): GatewayFrame | null {
  try {
    const frame = JSON.parse(value) as unknown;
    return typeof frame === "object" && frame !== null ? (frame as GatewayFrame) : null;
  } catch {
    return null;
  }
}

function restoreFailureFrame(id: string, message: string): string {
  return JSON.stringify({
    id,
    type: "error",
    action: "session.launch",
    payload: { code: "INTERNAL_ERROR", message },
  });
}

function isTargetWorkspaceRestore(
  frame: GatewayFrame | null,
  taskId: string,
  sessionId: string,
): frame is GatewayFrame & { id: string; payload: Record<string, unknown> } {
  return (
    frame?.type === "request" &&
    typeof frame.id === "string" &&
    frame.action === "session.launch" &&
    frame.payload?.intent === "restore_workspace" &&
    frame.payload.task_id === taskId &&
    frame.payload.session_id === sessionId
  );
}

/** Fail one automatic restore request, then forward all later requests normally. */
export async function failNextWorkspaceRestore(
  page: Page,
  taskId: string,
  sessionId: string,
  failureMessage = "workspace restore failed for e2e",
): Promise<{ wasConsumed: () => boolean }> {
  let pending = true;

  await page.routeWebSocket(/\/ws$/, (socket) => {
    const server = socket.connectToServer();
    socket.onMessage((message: SocketMessage) => {
      if (typeof message !== "string") {
        server.send(message);
        return;
      }

      const forwarded: string[] = [];
      for (const part of message.split("\n")) {
        const frame = parseGatewayFrame(part.trim());
        if (pending && isTargetWorkspaceRestore(frame, taskId, sessionId)) {
          pending = false;
          socket.send(restoreFailureFrame(frame.id, failureMessage));
          continue;
        }
        if (part.trim()) forwarded.push(part);
      }
      if (forwarded.length > 0) server.send(forwarded.join("\n"));
    });
    server.onMessage((message: SocketMessage) => socket.send(message));
  });

  return { wasConsumed: () => !pending };
}

export async function seedCompletedConversation(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      executor_profile_id: seedData.worktreeExecutorProfileId,
    },
  );
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");
  await waitForSessionDone(apiClient, task.id, task.session_id, "Waiting for initial conversation");
  await apiClient.seedTaskSession(task.id, {
    state: "COMPLETED",
    sessionId: task.session_id,
    agentProfileId: seedData.agentProfileId,
    repositoryId: seedData.repositoryId,
    completedAt: new Date().toISOString(),
  });
  await apiClient.updateTaskState(task.id, "COMPLETED");
  return task;
}

/** Restart the managed worker and prove the retained environment is cold but restorable. */
export async function restartAndAssertColdWorkspace(
  backend: BackendContext,
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
): Promise<void> {
  await backend.restart();

  const environment = await apiClient.getTaskEnvironment(taskId);
  expect(environment, "completed task must retain its workspace environment").not.toBeNull();
  expect(environment?.status).toBe("ready");

  const status = await apiClient.wsRequest<{
    state: string;
    is_agent_running: boolean;
    needs_workspace_restore: boolean;
  }>("task.session.status", { task_id: taskId, session_id: sessionId });
  expect(status.state).toBe("COMPLETED");
  expect(status.is_agent_running).toBe(false);
  expect(status.needs_workspace_restore).toBe(true);
}
