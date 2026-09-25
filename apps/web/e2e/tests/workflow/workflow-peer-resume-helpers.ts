import { expect } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { createWorkflowAgentProfiles } from "./workflow-agent-switch-helpers";

const PEER_OUTPUT_DELAY_MS = 12_000;

export type WorkflowPeerResumeScenario = {
  taskId: string;
  taskTitle: string;
  workflowId: string;
  reviewStepId: string;
  initialSessionId: string;
  implementationSessionId: string;
  implementationMarker: string;
  outputMarker: string;
  completionMarker: string;
  reviewMarker: string;
};

export async function waitForPeerSessionState(
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
  expectedState: string,
  timeoutMs = 30_000,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        return sessions.find((session) => session.id === sessionId)?.state ?? "";
      },
      {
        timeout: timeoutMs,
        message: `session ${sessionId} did not become ${expectedState}`,
      },
    )
    .toBe(expectedState);
}

async function waitForAgentMarker(
  apiClient: ApiClient,
  sessionId: string,
  marker: string,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        return messages.some(
          (message) => message.author_type === "agent" && message.content.includes(marker),
        );
      },
      { timeout: 30_000, message: `session ${sessionId} did not emit ${marker}` },
    )
    .toBe(true);
}

export async function createWorkflowPeerResumeScenario(
  apiClient: ApiClient,
  seedData: SeedData,
  name: string,
  options: { sendPeerMessage?: boolean } = {},
): Promise<WorkflowPeerResumeScenario> {
  const { profileA, profileB } = await createWorkflowAgentProfiles(apiClient);
  const workflow = await apiClient.createWorkflow(seedData.workspaceId, `${name} workflow`);
  const initial = await apiClient.createWorkflowStep(workflow.id, "Plan", 0, {
    is_start_step: true,
    agent_profile_id: profileA.id,
    profile_session_start_policy: "new",
    profile_session_end_policy: "park",
    events: { on_enter: [{ type: "auto_start_agent" }] },
  });
  const implementation = await apiClient.createWorkflowStep(workflow.id, "Implement", 1, {
    agent_profile_id: profileB.id,
    profile_session_start_policy: "new",
    profile_session_end_policy: "park",
    events: { on_enter: [{ type: "auto_start_agent" }] },
  });
  const review = await apiClient.createWorkflowStep(workflow.id, "Review", 2, {
    session_target: { kind: "initial" },
    profile_session_end_policy: "park",
    events: { on_enter: [{ type: "auto_start_agent" }] },
  });
  await apiClient.createWorkflowStep(workflow.id, "PR", 3, {
    session_target: { kind: "step", step_id: implementation.id },
    profile_session_end_policy: "park",
  });

  const initialMarker = `${name} initial ready`;
  const implementationMarker = `${name} implementation ready`;
  const reviewMarker = `${name} review ready`;
  const outputMarker = `${name} peer output`;
  const completionMarker = `${name} peer turn complete`;
  await apiClient.updateWorkflowStep(initial.id, { prompt: `e2e:message("${initialMarker}")` });
  await apiClient.updateWorkflowStep(implementation.id, {
    prompt: `e2e:message("${implementationMarker}")`,
  });
  await apiClient.updateWorkflowStep(review.id, { prompt: `e2e:message("${reviewMarker}")` });

  const taskTitle = `${name} target`;
  const task = await apiClient.createTaskWithAgent(seedData.workspaceId, taskTitle, profileA.id, {
    workflow_id: workflow.id,
    workflow_step_id: initial.id,
    repository_ids: [seedData.repositoryId],
  });
  const initialSessionId = await waitForProfileSession(apiClient, task.id, profileA.id);
  await waitForAgentMarker(apiClient, initialSessionId, initialMarker);
  await waitForPeerSessionState(apiClient, task.id, initialSessionId, "WAITING_FOR_INPUT");

  await apiClient.moveTask(task.id, workflow.id, implementation.id);
  const implementationSessionId = await waitForProfileSession(apiClient, task.id, profileB.id);
  await waitForAgentMarker(apiClient, implementationSessionId, implementationMarker);
  await waitForPeerSessionState(apiClient, task.id, implementationSessionId, "WAITING_FOR_INPUT");

  if (options.sendPeerMessage !== false) {
    const peerPrompt = [
      `e2e:message("${outputMarker}")`,
      `e2e:delay(${PEER_OUTPUT_DELAY_MS})`,
      `e2e:message("${completionMarker}")`,
    ].join("\n");
    const reviewPrompt = [
      `e2e:mcp:kandev:message_task_kandev(${JSON.stringify({
        task_id: task.id,
        session_id: implementationSessionId,
        prompt: peerPrompt,
      })})`,
      `e2e:message("${reviewMarker}")`,
    ].join("\n");
    await apiClient.updateWorkflowStep(review.id, { prompt: reviewPrompt });
  }
  await apiClient.moveTask(task.id, workflow.id, review.id);
  await waitForAgentMarker(apiClient, initialSessionId, reviewMarker);
  await waitForPeerSessionState(apiClient, task.id, initialSessionId, "WAITING_FOR_INPUT");

  return {
    taskId: task.id,
    taskTitle,
    workflowId: workflow.id,
    reviewStepId: review.id,
    initialSessionId,
    implementationSessionId,
    implementationMarker,
    outputMarker,
    completionMarker,
    reviewMarker,
  };
}

export type SessionLaunchCapture = {
  taskId?: string;
  sessionId?: string;
  intent?: string;
  activationSource?: string;
};

export function attachSessionLaunchCapture(page: Page): { requests: SessionLaunchCapture[] } {
  const requests: SessionLaunchCapture[] = [];
  page.on("websocket", (socket) => {
    socket.on("framesent", (event) => {
      if (typeof event.payload !== "string" || !event.payload.includes('"session.launch"')) return;
      try {
        const message = JSON.parse(event.payload) as {
          action?: string;
          payload?: {
            task_id?: string;
            session_id?: string;
            intent?: string;
            activation_source?: string;
          };
        };
        if (message.action !== "session.launch") return;
        requests.push({
          taskId: message.payload?.task_id,
          sessionId: message.payload?.session_id,
          intent: message.payload?.intent,
          activationSource: message.payload?.activation_source,
        });
      } catch {
        // Ignore non-JSON websocket frames.
      }
    });
  });
  return { requests };
}

async function waitForProfileSession(
  apiClient: ApiClient,
  taskId: string,
  profileId: string,
): Promise<string> {
  let sessionId = "";
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        const session = sessions.find((item) => item.agent_profile_id === profileId);
        sessionId = session?.id ?? "";
        return session?.state === "WAITING_FOR_INPUT";
      },
      { timeout: 30_000, message: `profile ${profileId} did not become answerable` },
    )
    .toBe(true);
  return sessionId;
}

export async function countPeerDeliveries(
  apiClient: ApiClient,
  sessionId: string,
  marker: string,
): Promise<number> {
  const { messages } = await apiClient.listSessionMessages(sessionId);
  return messages.filter(
    (message) => message.author_type === "user" && message.content.includes(marker),
  ).length;
}
