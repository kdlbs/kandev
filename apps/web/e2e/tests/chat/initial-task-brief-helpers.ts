import { expect, type Page, type SeedData } from "../../fixtures/test-base";
import type { BackendContext } from "../../fixtures/backend";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

export const INITIAL_TASK_BRIEF =
  "Initial task brief: preserve this long user supplied context while the prepared session starts. " +
  "Use the repository conventions, inspect the existing workflow, and keep the final change reviewable.";
export const INITIAL_TASK_INSTRUCTION =
  "Begin with the user instruction and report the first concrete step.";
export const FOLLOWUP_INSTRUCTION = "Follow up without repeating the original brief.";

type InitialTaskBriefFlowOptions = {
  testPage: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  title: string;
};

export async function runInitialTaskBriefFlow({
  testPage,
  apiClient,
  seedData,
  title,
}: InitialTaskBriefFlowOptions): Promise<void> {
  const task = await apiClient.createTask(seedData.workspaceId, title, {
    description: INITIAL_TASK_BRIEF,
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    agent_profile_id: seedData.agentProfileId,
    prepare_session: true,
    repository_ids: [seedData.repositoryId],
  });
  if (!task.session_id) throw new Error("prepared task did not return a session_id");

  const session = new SessionPage(testPage);
  await testPage.goto(`/t/${task.id}`);
  await session.waitForLoad();
  // Prepared sessions can spend longer in environment setup than ordinary
  // tasks when CI shards are starting together. Keep the wait bounded, but
  // allow the session's own hydration/reload recovery to finish.
  await session.waitForChatIdle({ timeout: 90_000 });

  await assertInitialTaskBriefChatFlow({
    testPage,
    apiClient,
    session,
    sessionId: task.session_id,
  });
}

type InitialTaskBriefRecoveryFlowOptions = InitialTaskBriefFlowOptions & {
  backend: BackendContext;
};

type PreparedSessionSnapshot = {
  id: string;
  state: string;
  executionId: string;
  preparationStatus: string;
};

function preparedSessionSnapshot(
  session: Awaited<ReturnType<ApiClient["listTaskSessions"]>>["sessions"][number] | undefined,
): PreparedSessionSnapshot {
  const preparation = session?.metadata?.prepare_result;
  const preparationStatus =
    typeof preparation === "object" && preparation !== null && "status" in preparation
      ? String(preparation.status)
      : "";
  return {
    id: session?.id ?? "",
    state: session?.state ?? "",
    executionId: session?.agent_execution_id ?? "",
    preparationStatus,
  };
}

async function waitForPreparedSession(
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
  message: string,
  expectedState?: string,
): Promise<void> {
  const expected = {
    id: sessionId,
    executionId: expect.stringMatching(/\S/),
    preparationStatus: "completed",
    ...(expectedState ? { state: expectedState } : {}),
  };
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        return preparedSessionSnapshot(sessions.find((candidate) => candidate.id === sessionId));
      },
      { timeout: 90_000, message },
    )
    .toMatchObject(expected);
}

export async function runInitialTaskBriefAfterRecovery(
  options: InitialTaskBriefRecoveryFlowOptions,
): Promise<void> {
  const previousSettings = await options.apiClient.getUserSettings();
  const preventAutoStartOnOpen =
    previousSettings.settings.prevent_auto_start_agent_on_open === true;
  // Keep the recovered session prompt-free while Chat opens after restart.
  await options.apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: true });
  try {
    await runInitialTaskBriefAfterRecoveryFlow(options);
  } finally {
    await options.apiClient.saveUserSettings({
      prevent_auto_start_agent_on_open: preventAutoStartOnOpen,
    });
  }
}

async function runInitialTaskBriefAfterRecoveryFlow({
  testPage,
  apiClient,
  seedData,
  backend,
  title,
}: InitialTaskBriefRecoveryFlowOptions): Promise<void> {
  const task = await apiClient.createTask(seedData.workspaceId, title, {
    description: INITIAL_TASK_BRIEF,
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    agent_profile_id: seedData.agentProfileId,
    prepare_session: true,
    repository_ids: [seedData.repositoryId],
  });
  if (!task.session_id) throw new Error("prepared task did not return a session_id");

  const sessionId = task.session_id;
  const session = new SessionPage(testPage);
  await testPage.goto(`/t/${task.id}`);
  await session.waitForLoad();
  await waitForPreparedSession(
    apiClient,
    task.id,
    sessionId,
    "Waiting for workspace preparation to finish before backend restart",
  );
  // Start the prepared workspace through the production resume path. This
  // starts the agent without a task-description turn, so the first Chat
  // submission remains the first user-authored prompt.
  const launch = await apiClient.launchSession({
    task_id: task.id,
    session_id: sessionId,
    intent: "resume",
  });
  expect(launch.session_id).toBe(sessionId);
  expect(launch.agent_execution_id).toMatch(/\S/);
  await session.waitForChatIdle({ timeout: 90_000 });
  await waitForPreparedSession(
    apiClient,
    task.id,
    sessionId,
    "Waiting for the prompt-free prepared session to become ready",
    "WAITING_FOR_INPUT",
  );
  const beforeRestart = await apiClient.listSessionMessages(sessionId);
  expect(beforeRestart.messages.filter((message) => message.author_type === "user")).toHaveLength(
    0,
  );

  await backend.restart();
  await testPage.reload();
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 90_000 });
  await waitForPreparedSession(
    apiClient,
    task.id,
    sessionId,
    "Waiting for the same prepared session to recover without a prompt",
    "WAITING_FOR_INPUT",
  );
  const afterRestart = await apiClient.listSessionMessages(sessionId);
  expect(afterRestart.messages.filter((message) => message.author_type === "user")).toHaveLength(0);

  await assertInitialTaskBriefChatFlow({ testPage, apiClient, session, sessionId });
  expect((await apiClient.getTask(task.id)).description).toBe(INITIAL_TASK_BRIEF);
}

async function assertInitialTaskBriefChatFlow({
  testPage,
  apiClient,
  session,
  sessionId,
}: {
  testPage: Page;
  apiClient: ApiClient;
  session: SessionPage;
  sessionId: string;
}): Promise<void> {
  await session.sendMessageViaButton(INITIAL_TASK_INSTRUCTION);

  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        return messages.filter((message) => message.author_type === "user");
      },
      { timeout: 30_000, message: "Waiting for the combined first prompt to persist" },
    )
    .toHaveLength(1);
  const firstStoredMessage = (await apiClient.listSessionMessages(sessionId)).messages.find(
    (message) => message.author_type === "user",
  );
  expect(firstStoredMessage?.content).toContain(INITIAL_TASK_BRIEF);
  expect(firstStoredMessage?.content).toContain(INITIAL_TASK_INSTRUCTION);

  const chat = session.activeChat();
  const userBubbles = chat.getByTestId("user-message-bubble");
  await expect(userBubbles).toHaveCount(1, { timeout: 15_000 });
  const firstBubble = userBubbles.first();
  await expect(firstBubble).toContainText(INITIAL_TASK_BRIEF);
  await expect(firstBubble).toContainText(INITIAL_TASK_INSTRUCTION);
  const renderedFirstPrompt = (await firstBubble.innerText()).replace(/\s+/g, " ").trim();
  expect(renderedFirstPrompt.split(INITIAL_TASK_BRIEF)).toHaveLength(2);
  expect(renderedFirstPrompt.split(INITIAL_TASK_INSTRUCTION)).toHaveLength(2);

  await testPage.reload();
  await session.waitForLoad();
  await expect(userBubbles).toHaveCount(1, { timeout: 15_000 });
  await expect(userBubbles.first()).toContainText(INITIAL_TASK_BRIEF);
  await expect(userBubbles.first()).toContainText(INITIAL_TASK_INSTRUCTION);

  await session.waitForChatIdle({ timeout: 60_000 });
  await session.sendMessageViaButton(FOLLOWUP_INSTRUCTION);
  await expect(userBubbles).toHaveCount(2, { timeout: 15_000 });
  await expect(userBubbles.nth(1)).toContainText(FOLLOWUP_INSTRUCTION);
  await expect(userBubbles.nth(1)).not.toContainText(INITIAL_TASK_BRIEF);

  const userMessages = await apiClient.listSessionMessages(sessionId);
  const storedUserMessages = userMessages.messages.filter(
    (message) => message.author_type === "user",
  );
  expect(storedUserMessages).toHaveLength(2);
  expect(storedUserMessages[0]?.content).toContain(INITIAL_TASK_BRIEF);
  expect(storedUserMessages[0]?.content).toContain(INITIAL_TASK_INSTRUCTION);
  expect(storedUserMessages[0]?.content.split(INITIAL_TASK_BRIEF)).toHaveLength(2);
  expect(storedUserMessages[0]?.content.split(INITIAL_TASK_INSTRUCTION)).toHaveLength(2);
  expect(storedUserMessages[1]?.content).toContain(FOLLOWUP_INSTRUCTION);
  expect(storedUserMessages[1]?.content).not.toContain(INITIAL_TASK_BRIEF);

  const hasDocumentOverflow = await testPage.evaluate(
    () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
  );
  expect(hasDocumentOverflow).toBe(false);
}
