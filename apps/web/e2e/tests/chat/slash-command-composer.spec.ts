import { type Locator, type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { seedAvailableCommands } from "../../helpers/session-store";
import { attachAvailableCommandsCapture } from "../../helpers/ws-capture";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import type { CreateTaskResponse } from "../../../lib/types/http";

const SLOW_COMMAND = {
  name: "slow",
  description: "Run a slow response",
  input_hint: "duration",
};

async function createReadyTask(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<CreateTaskResponse> {
  return apiClient.createTaskWithAgent(seedData.workspaceId, title, seedData.agentProfileId, {
    description: "/e2e:simple-message",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
}

async function openTaskChat(page: Page, taskId: string): Promise<SessionPage> {
  await page.goto(`/t/${taskId}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  return session;
}

function chatEditor(scope: Locator | Page): Locator {
  // Multiple TipTap instances can be mounted; always scope to the first visible one in scope.
  return scope.locator(".tiptap.ProseMirror:visible").first();
}

async function selectSlowCommandWithEnter(
  page: Page,
  editor: Locator,
  initialText = "",
): Promise<void> {
  await editor.click();
  await editor.fill(initialText);
  await editor.pressSequentially("/s");
  await expect(page.getByText("/slow", { exact: true })).toBeVisible({ timeout: 5_000 });
  await editor.press("Enter");
  await expect(editor).toHaveText(/\/slow/, { timeout: 5_000 });
  await expect(editor.getByTestId("slash-command-chip")).toHaveText("/slow", { timeout: 5_000 });
}

async function openQuickChatWithAgent(page: Page): Promise<Locator> {
  await page.goto("/");
  await page.waitForLoadState("networkidle");

  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await page.keyboard.press(`${modifier}+Shift+q`);

  const dialog = page.getByRole("dialog", { name: "Quick Chat" });
  await expect(dialog).toBeVisible({ timeout: 10_000 });

  const agentPicker = dialog.getByText("Choose an agent to start chatting");
  if (!(await agentPicker.isVisible({ timeout: 1_000 }).catch(() => false))) {
    await dialog.getByLabel("Start new chat").click();
  }
  await expect(agentPicker).toBeVisible({ timeout: 5_000 });

  const agentCard = dialog
    .locator("button")
    .filter({ has: page.locator(".rounded-md.border") })
    .first();
  await expect(agentCard).toBeVisible({ timeout: 5_000 });
  await agentCard.click();

  const editor = chatEditor(dialog);
  await expect(editor).toBeVisible({ timeout: 15_000 });
  await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 30_000 });
  return dialog;
}

test.describe("Slash command composer", () => {
  test("selecting a slash command keeps it as an editable draft until explicit send", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await createReadyTask(apiClient, seedData, "Slash Command Draft");
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

    const session = await openTaskChat(testPage, task.id);
    await seedAvailableCommands(testPage, task.session_id, [SLOW_COMMAND]);

    const editor = chatEditor(testPage);
    await selectSlowCommandWithEnter(testPage, editor);

    const chatList = session.chat.locator(".chat-message-list:visible");
    await expect(chatList.getByText("/slow", { exact: false })).not.toBeVisible({ timeout: 1_000 });
    await expect(chatList.getByText("Running slow response", { exact: false })).not.toBeVisible({
      timeout: 1_000,
    });

    await editor.pressSequentially("1s");
    await expect(editor).toHaveText(/\/slow\s+1s/, { timeout: 5_000 });
    await testPage.getByTestId("submit-message-button").click();

    await expect(chatList.getByText("/slow 1s", { exact: false })).toBeVisible({
      timeout: 10_000,
    });
    await expect(
      chatList.getByText("Slow response complete after 1s.", { exact: false }),
    ).toBeVisible({
      timeout: 30_000,
    });
  });

  test("selecting a slash command preserves existing draft text", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await createReadyTask(apiClient, seedData, "Slash Command Prefix Draft");
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

    const session = await openTaskChat(testPage, task.id);
    await seedAvailableCommands(testPage, task.session_id, [SLOW_COMMAND]);

    const editor = chatEditor(testPage);
    await selectSlowCommandWithEnter(testPage, editor, "please run ");

    await expect(editor).toHaveText(/please run \/slow/, { timeout: 5_000 });
    await expect(editor.getByTestId("slash-command-chip")).toHaveText("/slow", { timeout: 5_000 });
    await expect(
      session.chat.locator(".chat-message-list:visible").getByText("please run /slow", {
        exact: false,
      }),
    ).not.toBeVisible({ timeout: 1_000 });
  });

  test("escape closes the slash command menu without sending", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await createReadyTask(apiClient, seedData, "Slash Command Escape");
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

    const session = await openTaskChat(testPage, task.id);
    await seedAvailableCommands(testPage, task.session_id, [SLOW_COMMAND]);

    const editor = chatEditor(testPage);
    await editor.click();
    await editor.fill("");
    await editor.pressSequentially("/s");
    await expect(testPage.getByText("/slow", { exact: true })).toBeVisible({ timeout: 5_000 });

    await editor.press("Escape");

    await expect(testPage.getByText("/slow", { exact: true })).not.toBeVisible({
      timeout: 5_000,
    });
    await expect(
      session.chat.locator(".chat-message-list:visible").getByText("/slow", { exact: false }),
    ).not.toBeVisible({ timeout: 1_000 });
  });

  test("quick chat selection does not auto-send", async ({ testPage }) => {
    const availableCommands = attachAvailableCommandsCapture(testPage);
    const dialog = await openQuickChatWithAgent(testPage);
    await expect
      .poll(() => availableCommands.frames.some((frame) => frame.count > 0), { timeout: 15_000 })
      .toBe(true);

    const editor = chatEditor(dialog);

    await selectSlowCommandWithEnter(testPage, editor);

    await expect(editor).toHaveText(/\/slow/, { timeout: 5_000 });
    await expect(
      dialog.locator(".chat-message-list:visible").getByText("/slow", { exact: false }),
    ).not.toBeVisible({
      timeout: 1_000,
    });
  });
});
