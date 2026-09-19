import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { SessionPage } from "../pages/session-page";
import { waitForSessionDone, waitForSessionState } from "./session";

type SeedOptions = {
  /** Mock-agent scenario slug, e.g. "clarification" or "simple-message". */
  scenario: string;
  /**
   * Wait for the chat to go idle after load. Leave false for blocking
   * clarification scenarios where the agent parks on the MCP call and the idle
   * input never appears until the question is answered.
   */
  waitForIdle?: boolean;
};

/**
 * Create a task + session running a mock-agent scenario, wait for blocking
 * scenarios to reach their causal backend state, navigate to it, and return a
 * ready SessionPage. Shared by the clarification specs (overlay, resize, and
 * mobile) so the seed → wait → navigate → waitForLoad flow lives in one place.
 */
export async function seedClarificationSession(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  { scenario, waitForIdle = false }: SeedOptions,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: `/e2e:${scenario}`,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  if (!waitForIdle) {
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId: task.session_id,
      expectedState: "WAITING_FOR_INPUT",
      message: "clarification session should reach its blocking state before navigation",
      timeout: 60_000,
    });
  }

  await testPage.goto(`/t/${task.id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();
  if (waitForIdle) await session.waitForChatIdle();

  return session;
}

export type SecondaryClarificationTask = {
  id: string;
  title: string;
  primarySessionId: string;
  clarificationSessionId: string;
};

/**
 * Create a task with a clean primary session and a newer secondary session
 * blocked on clarification. The original session is restored as primary so
 * task navigation must follow pending ownership instead of its normal default.
 */
export async function seedSecondaryClarificationTask(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<SecondaryClarificationTask> {
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
  if (!task.session_id) throw new Error("secondary clarification setup has no primary session");

  await waitForSessionDone(
    apiClient,
    task.id,
    task.session_id,
    "primary session should finish before secondary clarification starts",
    60_000,
  );
  const secondary = await apiClient.launchSession(
    {
      task_id: task.id,
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
      workflow_step_id: seedData.startStepId,
      prompt: "/e2e:clarification",
    },
    60_000,
  );
  await waitForSessionState(apiClient, {
    taskId: task.id,
    sessionId: secondary.session_id,
    expectedState: "WAITING_FOR_INPUT",
    message: "secondary session should wait on clarification",
    timeout: 60_000,
  });

  await apiClient.setPrimarySession(task.session_id);
  await expect
    .poll(async () => (await apiClient.getTask(task.id)).primary_session_id ?? null, {
      message: "clean original session should remain primary",
      timeout: 30_000,
    })
    .toBe(task.session_id);
  await expect
    .poll(
      async () => {
        const { tasks } = await apiClient.listTasks(seedData.workspaceId);
        return tasks.find((candidate) => candidate.id === task.id)?.status_summary?.pending_action;
      },
      { message: "task summary should advertise secondary clarification", timeout: 30_000 },
    )
    .toBe("clarification");

  return {
    id: task.id,
    title,
    primarySessionId: task.session_id,
    clarificationSessionId: secondary.session_id,
  };
}

export async function activeSessionId(page: Page): Promise<string | null> {
  return page.evaluate(
    () =>
      (
        window as unknown as {
          __KANDEV_E2E_STORE__?: { getState: () => { tasks: { activeSessionId: string | null } } };
        }
      ).__KANDEV_E2E_STORE__?.getState().tasks.activeSessionId ?? null,
  );
}
