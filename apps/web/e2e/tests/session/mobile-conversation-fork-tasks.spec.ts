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
  destination: "task" | "child_task",
) {
  const cutoff = session
    .activeChat()
    .getByTestId("agent-message-highlight")
    .filter({ hasText: FORK_SOURCE_ASSISTANT });
  await expect(cutoff).toBeVisible();
  await cutoff.getByTestId("conversation-fork-message-action").tap();
  const picker = testPage.getByTestId("conversation-fork-picker");
  await expect(picker).toBeVisible();
  await picker.getByTestId(`conversation-fork-destination-${destination}`).tap();
  await picker.getByRole("button", { name: "Continue" }).tap();
}

function taskIdFromUrl(url: string): string {
  const taskId = url.match(/\/t\/([^/?]+)/)?.[1];
  if (!taskId) throw new Error(`expected task URL, received ${url}`);
  return taskId;
}

test.describe("Conversation forks into tasks on phone", () => {
  test("keeps a create-only new-task fork after reload and starts its frozen history", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(240_000);
    const source = await seedConversationForkSource(
      apiClient,
      seedData,
      "Mobile conversation fork task source",
    );
    await testPage.goto(`/t/${source.taskId}`);
    const sourcePage = new SessionPage(testPage);
    await sourcePage.waitForLoad();
    await sourcePage.waitForChatIdle({ timeout: 30_000 });
    await chooseForkDestination(testPage, sourcePage, "task");

    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    const viewport = testPage.viewportSize();
    expect(viewport).not.toBeNull();
    await expect
      .poll(async () => (await dialog.boundingBox())?.height ?? 0)
      .toBeGreaterThanOrEqual(viewport!.height - 4);
    const dialogBox = await dialog.boundingBox();
    expect(dialogBox).not.toBeNull();
    expect(dialogBox!.height).toBeGreaterThanOrEqual(viewport!.height - 4);
    expect(dialogBox!.x).toBeGreaterThanOrEqual(0);
    expect(dialogBox!.x + dialogBox!.width).toBeLessThanOrEqual(viewport!.width);

    const chip = dialog.getByTestId("conversation-fork-chip");
    await expect(chip).toContainText("Mobile conversation fork task source");
    const previewButton = chip.getByRole("button", { name: "Preview" });
    const previewBox = await previewButton.boundingBox();
    expect(previewBox?.height).toBeGreaterThanOrEqual(44);
    await previewButton.tap();
    const preview = testPage.getByTestId("conversation-fork-preview-overlay");
    await expect(preview.getByTestId("conversation-fork-content")).toContainText(FORK_SOURCE_USER);
    await expect
      .poll(async () => (await preview.boundingBox())?.height ?? 0)
      .toBeGreaterThanOrEqual(viewport!.height - 4);
    const previewBoxOnPhone = await preview.boundingBox();
    expect(previewBoxOnPhone).not.toBeNull();
    expect(previewBoxOnPhone!.height).toBeGreaterThanOrEqual(viewport!.height - 4);
    await testPage.getByRole("button", { name: "Back" }).tap();

    const title = `Mobile forked task ${Date.now()}`;
    await dialog.getByTestId("task-title-input").fill(title);
    await dialog.getByTestId("task-description-input").fill(FORK_NEW_INSTRUCTION);
    await expect(dialog.getByTestId("mobile-create-without-agent")).toBeEnabled({
      timeout: 30_000,
    });
    await dialog.getByTestId("mobile-create-without-agent").tap();
    await expect(dialog).not.toBeVisible({ timeout: 15_000 });
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const taskId = taskIdFromUrl(testPage.url());
    const task = await apiClient.getTask(taskId);
    expect(task.parent_id).toBeFalsy();
    expect(task.title).toBe(title);
    const forkId = task.metadata?.conversation_fork_id;
    expect(typeof forkId).toBe("string");
    const frozen = await apiClient.getConversationForkContent(forkId as string);
    expect(frozen.content).toContain(FORK_SOURCE_USER);
    expect(frozen.content).not.toContain(FORK_SOURCE_AFTER_CUTOFF);

    await testPage.reload();
    const destinationPage = new SessionPage(testPage);
    await destinationPage.waitForLoad();
    const provenance = testPage.getByTestId("task-conversation-fork-provenance");
    await expect(provenance).toBeVisible();
    await provenance.getByTestId("conversation-fork-provenance-trigger").tap();
    await expect(testPage.getByTestId("conversation-fork-provenance-content")).toContainText(
      FORK_SOURCE_USER,
    );
    await testPage.keyboard.press("Escape");
    await testPage.getByTestId("task-description-start-button").tap();

    const sessions = await apiClient.listTaskSessions(taskId);
    const sessionId = sessions.sessions[0]?.id;
    expect(sessionId).toBeTruthy();
    await waitForSessionDone(
      apiClient,
      taskId,
      sessionId!,
      "Waiting for the mobile delayed fork turn",
    );
    const { messages } = await apiClient.listSessionMessages(sessionId!);
    const firstUserMessage = messages.find((message) => message.author_type === "user");
    expect(firstUserMessage?.content).toContain(FORK_NEW_INSTRUCTION);
    expect(firstUserMessage?.metadata?.conversation_fork_id).toBe(forkId);
    expect((await apiClient.getConversationForkContent(forkId as string)).content).toBe(
      frozen.content,
    );
  });

  test("offers a separate execution workspace for a child fork on phone", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(240_000);
    const source = await seedConversationForkSource(
      apiClient,
      seedData,
      "Mobile conversation fork child source",
    );
    await testPage.goto(`/t/${source.taskId}`);
    const sourcePage = new SessionPage(testPage);
    await sourcePage.waitForLoad();
    await sourcePage.waitForChatIdle({ timeout: 30_000 });
    await chooseForkDestination(testPage, sourcePage, "child_task");

    const dialog = testPage.getByTestId("new-subtask-dialog");
    await expect(dialog).toBeVisible();
    const viewport = testPage.viewportSize();
    expect(viewport).not.toBeNull();
    await expect
      .poll(async () => (await dialog.boundingBox())?.height ?? 0)
      .toBeGreaterThanOrEqual(viewport!.height - 20);
    const dialogBox = await dialog.boundingBox();
    expect(dialogBox).not.toBeNull();
    expect(dialogBox!.height).toBeGreaterThanOrEqual(viewport!.height - 20);
    await expect(dialog.getByTestId("conversation-fork-chip")).toContainText(
      "Mobile conversation fork child source",
    );

    const sharedOption = dialog.getByTestId("subtask-workspace-mode-inherit");
    const separateOption = dialog.getByTestId("subtask-workspace-mode-new");
    await expect(sharedOption).toBeVisible();
    await expect(separateOption).toBeVisible();
    const separateBox = await separateOption.boundingBox();
    expect(separateBox?.height).toBeGreaterThanOrEqual(44);
    await separateOption.tap();
    await expect(separateOption).toHaveAttribute("aria-checked", "true");
    const { executors } = await apiClient.listExecutors();
    const worktreeProfile = executors
      .flatMap((executor) => executor.profiles ?? [])
      .find((profile) => profile.id === seedData.worktreeExecutorProfileId);
    expect(worktreeProfile).toBeDefined();
    const executorSelector = dialog.getByTestId("executor-profile-selector");
    await executorSelector.tap();
    const worktreeOption = testPage.getByRole("option").filter({ hasText: worktreeProfile!.name });
    await expect(worktreeOption).toBeVisible();
    await worktreeOption.tap();
    await expect(executorSelector).toContainText(worktreeProfile!.name);
    await dialog.getByTestId("subtask-title-input").fill("Mobile forked child in new workspace");
    await dialog.getByTestId("subtask-prompt-input").fill(FORK_NEW_INSTRUCTION);
    await dialog.getByRole("button", { name: "Create subtask" }).tap();
    await expect(dialog).not.toBeVisible({ timeout: 15_000 });
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const childId = taskIdFromUrl(testPage.url());
    const child = await apiClient.getTask(childId);
    expect(child.parent_id).toBe(source.taskId);
    const workspace = child.metadata?.workspace as { mode?: string } | undefined;
    expect(workspace?.mode).toBe("new_workspace");
    const forkId = child.metadata?.conversation_fork_id;
    expect(typeof forkId).toBe("string");
    const frozen = await apiClient.getConversationForkContent(forkId as string);
    expect(frozen.content).toContain(FORK_SOURCE_USER);
    expect(frozen.content).toContain(FORK_SOURCE_ASSISTANT);
    expect(frozen.content).not.toContain(FORK_SOURCE_AFTER_CUTOFF);

    const sessions = await apiClient.listTaskSessions(childId);
    const sessionId = sessions.sessions[0]?.id;
    expect(sessionId).toBeTruthy();
    await waitForSessionDone(
      apiClient,
      childId,
      sessionId!,
      "Waiting for the mobile new-workspace child turn",
    );
    const { messages } = await apiClient.listSessionMessages(sessionId!);
    const firstUserMessage = messages.find((message) => message.author_type === "user");
    expect(firstUserMessage?.content).toContain(FORK_NEW_INSTRUCTION);
    expect(firstUserMessage?.metadata?.conversation_fork_id).toBe(forkId);
    expect((await apiClient.getConversationForkContent(forkId as string)).content).toBe(
      frozen.content,
    );
  });
});
