import { expect, test } from "../../fixtures/test-base";
import { waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import {
  FORK_NEW_INSTRUCTION,
  FORK_SOURCE_AFTER_CUTOFF,
  FORK_SOURCE_ASSISTANT,
  FORK_SOURCE_USER,
  seedConversationForkSource,
} from "./conversation-fork-helpers";

async function chooseForkDestination(
  testPage: import("@playwright/test").Page,
  session: SessionPage,
  messageText: string,
  destination: "task" | "child_task",
) {
  const cutoff = session
    .activeChat()
    .getByTestId("agent-message-highlight")
    .filter({ hasText: messageText });
  await expect(cutoff).toBeVisible();
  const action = cutoff.getByTestId("conversation-fork-message-action");
  await action.hover();
  await action.click();

  const picker = testPage.getByTestId("conversation-fork-picker");
  await expect(picker).toBeVisible();
  await picker.getByTestId(`conversation-fork-destination-${destination}`).click();
  await picker.getByRole("button", { name: "Continue" }).click();
}

function taskIdFromUrl(url: string): string {
  const taskId = url.match(/\/t\/([^/?]+)/)?.[1];
  if (!taskId) throw new Error(`expected task URL, received ${url}`);
  return taskId;
}

test.describe("Conversation forks into tasks", () => {
  test("keeps a create-only fork through reload and its later first launch", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(240_000);
    const source = await seedConversationForkSource(
      apiClient,
      seedData,
      "Conversation fork delayed task source",
    );
    await testPage.goto(`/t/${source.taskId}`);
    const sourcePage = new SessionPage(testPage);
    await sourcePage.waitForLoad();
    await sourcePage.waitForChatIdle({ timeout: 30_000 });
    await chooseForkDestination(testPage, sourcePage, FORK_SOURCE_ASSISTANT, "task");

    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    const chip = dialog.getByTestId("conversation-fork-chip");
    await expect(chip).toContainText("Conversation fork delayed task source");
    await expect(chip).toContainText("Full conversation");
    const title = `Forked delayed task ${Date.now()}`;
    await dialog.getByTestId("task-title-input").fill(title);
    await dialog.getByTestId("task-description-input").fill(FORK_NEW_INSTRUCTION);
    await expect(dialog.getByTestId("submit-start-agent")).toBeEnabled({ timeout: 30_000 });
    await dialog.getByTestId("submit-start-agent-chevron").click();
    await testPage.getByTestId("submit-create-without-agent").click();
    await expect(dialog).not.toBeVisible({ timeout: 15_000 });
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const destinationTaskId = taskIdFromUrl(testPage.url());
    const taskBeforeReload = await apiClient.getTask(destinationTaskId);
    expect(taskBeforeReload.title).toBe(title);
    expect(taskBeforeReload.parent_id).toBeFalsy();
    const forkId = taskBeforeReload.metadata?.conversation_fork_id;
    expect(typeof forkId).toBe("string");
    const frozen = await apiClient.getConversationForkContent(forkId as string);
    expect(frozen.content).toContain(FORK_SOURCE_USER);
    expect(frozen.content).toContain(FORK_SOURCE_ASSISTANT);
    expect(frozen.content).not.toContain(FORK_SOURCE_AFTER_CUTOFF);

    const sessionsBeforeReload = await apiClient.listTaskSessions(destinationTaskId);
    expect(sessionsBeforeReload.sessions).toHaveLength(1);
    expect(sessionsBeforeReload.sessions[0]?.state).toBe("CREATED");
    await testPage.reload();
    const destinationPage = new SessionPage(testPage);
    await destinationPage.waitForLoad();
    const provenance = testPage.getByTestId("task-conversation-fork-provenance");
    await expect(provenance).toBeVisible();
    await provenance.getByTestId("conversation-fork-provenance-trigger").click();
    await expect(testPage.getByTestId("conversation-fork-provenance-content")).toContainText(
      FORK_SOURCE_USER,
    );
    await testPage.keyboard.press("Escape");

    await testPage.getByTestId("task-description-start-button").click();
    const destinationSessionId = sessionsBeforeReload.sessions[0]!.id;
    await waitForSessionDone(
      apiClient,
      destinationTaskId,
      destinationSessionId,
      "Waiting for the delayed forked task turn",
    );
    const { messages } = await apiClient.listSessionMessages(destinationSessionId);
    const firstUserMessage = messages.find((message) => message.author_type === "user");
    expect(firstUserMessage?.content).toContain(FORK_NEW_INSTRUCTION);
    expect(firstUserMessage?.metadata?.conversation_fork_id).toBe(forkId);
    const delivered = await apiClient.getConversationForkContent(forkId as string);
    expect(delivered.content).toBe(frozen.content);
  });

  test("creates a child task in the selected shared workspace with the frozen range", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(240_000);
    const source = await seedConversationForkSource(
      apiClient,
      seedData,
      "Conversation fork shared child source",
    );
    await testPage.goto(`/t/${source.taskId}`);
    const sourcePage = new SessionPage(testPage);
    await sourcePage.waitForLoad();
    await sourcePage.waitForChatIdle({ timeout: 30_000 });
    await chooseForkDestination(testPage, sourcePage, FORK_SOURCE_ASSISTANT, "child_task");

    const dialog = testPage.getByTestId("new-subtask-dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText("Conversation fork shared child source");
    const chip = dialog.getByTestId("conversation-fork-chip");
    await expect(chip).toContainText("Conversation fork shared child source");
    await dialog.getByTestId("subtask-workspace-mode-inherit").click();
    await expect(dialog.getByTestId("subtask-workspace-mode-inherit")).toHaveAttribute(
      "aria-checked",
      "true",
    );
    await dialog.getByTestId("subtask-title-input").fill("Forked shared child task");
    await dialog.getByTestId("subtask-prompt-input").fill(FORK_NEW_INSTRUCTION);
    await dialog.getByRole("button", { name: "Create subtask" }).click();
    await expect(dialog).not.toBeVisible({ timeout: 15_000 });
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const childId = taskIdFromUrl(testPage.url());
    const child = await apiClient.getTask(childId);
    expect(child.parent_id).toBe(source.taskId);
    const workspace = child.metadata?.workspace as { mode?: string } | undefined;
    expect(workspace?.mode).toBe("inherit_parent");
    const forkId = child.metadata?.conversation_fork_id;
    expect(typeof forkId).toBe("string");
    const frozen = await apiClient.getConversationForkContent(forkId as string);
    expect(frozen.content).toContain(FORK_SOURCE_USER);
    expect(frozen.content).toContain(FORK_SOURCE_ASSISTANT);
    expect(frozen.content).not.toContain(FORK_SOURCE_AFTER_CUTOFF);

    const childSessions = await apiClient.listTaskSessions(childId);
    const childSession = childSessions.sessions[0];
    expect(childSession).toBeTruthy();
    await waitForSessionDone(
      apiClient,
      childId,
      childSession!.id,
      "Waiting for the shared child turn",
    );
    const { messages } = await apiClient.listSessionMessages(childSession!.id);
    const firstUserMessage = messages.find((message) => message.author_type === "user");
    expect(firstUserMessage?.content).toContain(FORK_NEW_INSTRUCTION);
    expect(firstUserMessage?.metadata?.conversation_fork_id).toBe(forkId);
    expect((await apiClient.getConversationForkContent(forkId as string)).content).toBe(
      frozen.content,
    );
  });
});
