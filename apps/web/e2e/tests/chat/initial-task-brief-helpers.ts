import { expect, type Page, type SeedData } from "../../fixtures/test-base";
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
  await session.sendMessageViaButton(INITIAL_TASK_INSTRUCTION);

  const chat = session.activeChat();
  const userBubbles = chat.getByTestId("user-message-bubble");
  await expect(userBubbles).toHaveCount(1, { timeout: 15_000 });
  const firstBubble = userBubbles.first();
  await expect(firstBubble).toContainText(INITIAL_TASK_BRIEF);
  await expect(firstBubble).toContainText(INITIAL_TASK_INSTRUCTION);

  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(task.session_id!);
        return messages.filter((message) => message.author_type === "user");
      },
      { timeout: 30_000, message: "Waiting for the combined first prompt to persist" },
    )
    .toHaveLength(1);

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

  const userMessages = await apiClient.listSessionMessages(task.session_id);
  const storedUserMessages = userMessages.messages.filter(
    (message) => message.author_type === "user",
  );
  expect(storedUserMessages).toHaveLength(2);
  expect(storedUserMessages[0]?.content).toContain(INITIAL_TASK_BRIEF);
  expect(storedUserMessages[0]?.content).toContain(INITIAL_TASK_INSTRUCTION);
  expect(storedUserMessages[1]?.content).toContain(FOLLOWUP_INSTRUCTION);
  expect(storedUserMessages[1]?.content).not.toContain(INITIAL_TASK_BRIEF);

  const hasDocumentOverflow = await testPage.evaluate(
    () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
  );
  expect(hasDocumentOverflow).toBe(false);
}
