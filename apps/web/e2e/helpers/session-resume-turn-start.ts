import { expect, type Page } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { CreateTaskResponse } from "../../lib/types/http";
import type { ApiClient, QueueSessionIdentityInput } from "./api-client";
import { SessionPage } from "../pages/session-page";
import {
  createDelayedResumeProfile,
  readSessionMessageIdsContaining,
  readSessionRuntimeIdentity,
  waitForSessionStarting,
  type SessionRuntimeIdentity,
} from "./session-resume-prompt-queue";

export type ResumeTurnStartFixture = {
  task: CreateTaskResponse;
  session: SessionPage;
  identity: QueueSessionIdentityInput;
  delayedProfileId: string;
  workflowId: string;
  startStepId: string;
  destinationStepId: string;
  savedRuntime: SessionRuntimeIdentity;
};

type E2EStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => {
      taskSessions: { items: Record<string, { state?: string } | undefined> };
    };
  };
};

async function getBrowserSessionState(page: Page, sessionId: string): Promise<string | null> {
  return page.evaluate((id) => {
    const state = (window as E2EStoreWindow).__KANDEV_E2E_STORE__?.getState();
    return state?.taskSessions.items[id]?.state ?? null;
  }, sessionId);
}

async function getSessionState(
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
): Promise<string | null> {
  const { sessions } = await apiClient.listTaskSessions(taskId);
  return sessions.find((session) => session.id === sessionId)?.state ?? null;
}

/** Seed a saved conversation, then resume it with an armed turn-start workflow transition. */
export async function seedResumeTurnStartFixture(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  backend: BackendContext,
  title: string,
): Promise<ResumeTurnStartFixture> {
  const delayedProfileId = await createDelayedResumeProfile(apiClient);
  let workflowId = "";
  let task: CreateTaskResponse | undefined;

  try {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, `${title} Workflow`);
    workflowId = workflow.id;
    const startStep = await apiClient.createWorkflowStep(workflowId, "Resume Pending", 0, {
      is_start_step: true,
      agent_profile_id: delayedProfileId,
    });
    const destinationStep = await apiClient.createWorkflowStep(
      workflowId,
      "Resume Destination",
      1,
      { agent_profile_id: delayedProfileId },
    );
    task = await apiClient.createTaskWithAgent(seedData.workspaceId, title, delayedProfileId, {
      description: "/e2e:simple-message",
      workflow_id: workflowId,
      workflow_step_id: startStep.id,
      repository_ids: [seedData.repositoryId],
    });
    if (!task.session_id) throw new Error("turn-start resume task has no session_id");

    await page.goto(`/t/${task.id}`);
    const session = new SessionPage(page);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 60_000 });
    const savedRuntime = await readSessionRuntimeIdentity(apiClient, task.id, task.session_id);

    // Arm the transition only after the first prompt has established a saved conversation.
    await apiClient.updateWorkflowStep(startStep.id, {
      events: { on_turn_start: [{ type: "move_to_next" }] },
    });

    await backend.restart();
    await page.reload();
    await session.waitForLoad();
    await expect
      .poll(() => getSessionState(apiClient, task!.id, task!.session_id!), {
        timeout: 30_000,
        message: `session ${task.session_id} should enter startup after restart`,
      })
      .toBe("STARTING");
    await page.reload();
    await session.waitForLoad();
    await waitForSessionStarting(page, apiClient, task.id, task.session_id, 30_000);
    const identity = await apiClient.getQueueSessionIdentity(task.id, task.session_id);

    return {
      task,
      session,
      identity,
      delayedProfileId,
      workflowId,
      startStepId: startStep.id,
      destinationStepId: destinationStep.id,
      savedRuntime,
    };
  } catch (error) {
    if (task) {
      await apiClient.deleteTask(task.id, { discardWorktreeChanges: true }).catch(() => undefined);
    }
    if (workflowId) await apiClient.deleteWorkflow(workflowId).catch(() => undefined);
    await apiClient.deleteAgentProfile(delayedProfileId, true).catch(() => undefined);
    throw error;
  }
}

export async function cleanupResumeTurnStartFixture(
  apiClient: ApiClient,
  fixture: ResumeTurnStartFixture,
): Promise<void> {
  await apiClient
    .deleteTask(fixture.task.id, { discardWorktreeChanges: true })
    .catch(() => undefined);
  await apiClient.deleteWorkflow(fixture.workflowId).catch(() => undefined);
  await apiClient.deleteAgentProfile(fixture.delayedProfileId, true).catch(() => undefined);
}

export async function waitForResumeTurnStartTransition(
  apiClient: ApiClient,
  fixture: ResumeTurnStartFixture,
  count: number,
  timeout = 30_000,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const { history } = await apiClient.listWorkflowHistory(fixture.identity.sessionId);
        return (history ?? []).filter(
          (transition) =>
            transition.from_step_id === fixture.startStepId &&
            transition.to_step_id === fixture.destinationStepId &&
            transition.trigger === "on_turn_start",
        );
      },
      {
        timeout,
        message: `session ${fixture.identity.sessionId} should record ${count} turn-start transition(s)`,
      },
    )
    .toHaveLength(count);
}

export async function waitForResumeQueuedPrompt(
  apiClient: ApiClient,
  fixture: ResumeTurnStartFixture,
  marker: string,
  timeout = 20_000,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const status = await apiClient.getQueueStatus(fixture.identity);
        return status.entries.filter((entry) => entry.content.includes(marker)).length;
      },
      {
        timeout,
        message: `session ${fixture.identity.sessionId} should retain queued prompt ${marker}`,
      },
    )
    .toBe(1);
}

export async function waitForResumeSessionReady(
  page: Page,
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
  timeout = 60_000,
): Promise<void> {
  const promptable = new Set(["RUNNING", "WAITING_FOR_INPUT"]);
  await expect
    .poll(
      async () => {
        const api = await getSessionState(apiClient, taskId, sessionId);
        const browser = await getBrowserSessionState(page, sessionId);
        return promptable.has(api ?? "") && promptable.has(browser ?? "");
      },
      {
        timeout,
        message: `session ${sessionId} should become promptable after resume`,
      },
    )
    .toBe(true);
}

export async function captureTurnStartMessageBaseline(
  apiClient: ApiClient,
  fixture: ResumeTurnStartFixture,
  marker: string,
): Promise<Set<string>> {
  return readSessionMessageIdsContaining(apiClient, fixture.identity.sessionId, marker);
}

export async function assertOneTurnStartPromptAndResponse(
  apiClient: ApiClient,
  fixture: ResumeTurnStartFixture,
  marker: string,
): Promise<void> {
  const [{ messages }, { sessions }] = await Promise.all([
    apiClient.listSessionMessages(fixture.identity.sessionId),
    apiClient.listTaskSessions(fixture.task.id),
  ]);
  const matchingUserMessages = messages.filter(
    (message) => message.author_type === "user" && message.content.includes(marker),
  );
  const matchingResponses = messages.filter(
    (message) =>
      message.author_type === "agent" &&
      message.type !== "error" &&
      message.content.includes(marker),
  );
  expect(matchingUserMessages).toHaveLength(1);
  expect(matchingUserMessages[0]?.turn_id).toBeTruthy();
  expect(matchingResponses).toHaveLength(1);
  expect(
    messages.filter((message) => message.type === "error" && message.content.includes(marker)),
  ).toHaveLength(0);

  const session = sessions.find((candidate) => candidate.id === fixture.identity.sessionId);
  expect(session?.error_message ?? "").toBe("");
  const runtime = await readSessionRuntimeIdentity(
    apiClient,
    fixture.task.id,
    fixture.identity.sessionId,
  );
  expect(runtime.executionId).toBe(session?.agent_execution_id);
  expect(runtime.acpSessionId).toBe(fixture.savedRuntime.acpSessionId);
}
