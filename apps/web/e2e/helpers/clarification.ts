import { expect, type Locator, type Page } from "@playwright/test";
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

export type SupersededClarificationStatus = "answered" | "rejected";

export const SUPERSEDED_OLD_QUESTION_PROMPT = "Which legacy database should we migrate first?";
export const SUPERSEDED_CURRENT_QUESTION_PROMPT =
  "Which environment should the new deployment use?";

export type SupersededClarificationFixture = {
  taskId: string;
  sessionId: string;
  currentTurnId: string;
  oldQuestionPrompt: string;
  currentQuestionPrompt: string;
};

/**
 * Seeds a completed older turn with a detached pending question A and a newer
 * open turn whose question B already resolved. The backend projection is clean,
 * so the chat must keep A as history instead of reactivating it.
 */
export async function seedSupersededClarification(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  status: SupersededClarificationStatus,
): Promise<SupersededClarificationFixture> {
  const task = await apiClient.createTask(seedData.workspaceId, title, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
    state: "RUNNING",
  });

  const now = Date.now();
  const oldTurnStartedAt = new Date(now - 240_000).toISOString();
  const oldTurnCompletedAt = new Date(now - 230_000).toISOString();
  const currentTurnStartedAt = new Date(now - 120_000).toISOString();
  const currentQuestionCreatedAt = new Date(now - 110_000).toISOString();
  const currentActivityCreatedAt = new Date(now - 60_000).toISOString();

  await apiClient.seedSessionMessage(sessionId, {
    type: "clarification_request",
    createdAt: oldTurnStartedAt,
    newTurn: true,
    turnStartedAt: oldTurnStartedAt,
    turnCompletedAt: oldTurnCompletedAt,
    metadata: {
      pending_id: "pending-superseded-old",
      session_id: sessionId,
      question_id: "q-superseded-old",
      question_index: 0,
      question_total: 1,
      status: "pending",
      agent_disconnected: true,
      question: {
        id: "q-superseded-old",
        title: "Legacy migration",
        prompt: SUPERSEDED_OLD_QUESTION_PROMPT,
        options: [
          { option_id: "opt-old-staging", label: "Legacy staging", description: "" },
          { option_id: "opt-old-production", label: "Legacy production", description: "" },
        ],
      },
    },
  });

  const currentQuestion = await apiClient.seedSessionMessage(sessionId, {
    type: "clarification_request",
    createdAt: currentQuestionCreatedAt,
    newTurn: true,
    turnStartedAt: currentTurnStartedAt,
    metadata: {
      pending_id: "pending-superseded-current",
      session_id: sessionId,
      question_id: "q-superseded-current",
      question_index: 0,
      question_total: 1,
      status,
      ...(status === "answered"
        ? {
            response: {
              question_id: "q-superseded-current",
              selected_options: ["opt-new-staging"],
            },
          }
        : {}),
      question: {
        id: "q-superseded-current",
        title: "Deployment target",
        prompt: SUPERSEDED_CURRENT_QUESTION_PROMPT,
        options: [
          { option_id: "opt-new-staging", label: "New staging", description: "" },
          { option_id: "opt-new-production", label: "New production", description: "" },
        ],
      },
    },
  });
  if (!currentQuestion.turnId) throw new Error("current question must belong to an open turn");

  await apiClient.seedSessionMessage(sessionId, {
    type: "message",
    content: "The newer turn continues with ordinary activity.",
    createdAt: currentActivityCreatedAt,
  });

  return {
    taskId: task.id,
    sessionId,
    currentTurnId: currentQuestion.turnId,
    oldQuestionPrompt: SUPERSEDED_OLD_QUESTION_PROMPT,
    currentQuestionPrompt: SUPERSEDED_CURRENT_QUESTION_PROMPT,
  };
}

export async function expectCleanSupersededProjection(
  apiClient: ApiClient,
  sessionId: string,
): Promise<void> {
  await expect
    .poll(async () => (await apiClient.getTaskSession(sessionId)).session.pending_action, {
      message: "the resolved newer question must leave a clean session projection",
      timeout: 30_000,
    })
    .toBeNull();
}

export async function expectNoActiveSupersededQuestion(
  page: Page,
  fixture: SupersededClarificationFixture,
): Promise<{ session: SessionPage; historical: Locator }> {
  const session = new SessionPage(page);
  await session.waitForLoad();
  await expect(session.clarificationBar()).toBeHidden();
  await expect(
    session
      .activeChat()
      .locator('[data-placeholder="Queue instructions while the question is pending..."]'),
  ).toHaveCount(0);
  await expect(session.activeChat().getByTestId("chat-input-area")).toBeVisible();

  const historical = session
    .activeChat()
    .getByTestId("clarification-request-message")
    .filter({ hasText: fixture.oldQuestionPrompt });
  await expect(historical).toBeVisible();
  await expect(
    session
      .activeChat()
      .getByTestId("clarification-request-message")
      .filter({ hasText: fixture.currentQuestionPrompt }),
  ).toBeVisible();
  return { session, historical };
}
