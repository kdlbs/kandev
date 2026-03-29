import { type Locator, type Page } from "@playwright/test";
import { test, expect } from "../fixtures/test-base";

/**
 * Quick Chat queued message E2E tests.
 *
 * Isolated into their own file because the agent stays busy for 10s during the test,
 * which can interfere with other quick chat tests sharing the same backend worker.
 */

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

  await expect(dialog.locator(".tiptap.ProseMirror")).toBeVisible({ timeout: 15_000 });
  return dialog;
}

/**
 * Type text into the TipTap editor while the agent is busy.
 * fill() silently fails on TipTap when the busy placeholder is shown,
 * so we retry clicking and typing until text appears in the editor.
 */
async function typeWhileBusy(page: Page, editor: Locator, text: string): Promise<void> {
  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await editor.scrollIntoViewIfNeeded();
  for (let attempt = 0; attempt < 3; attempt++) {
    const box = await editor.boundingBox();
    if (!box) throw new Error("Editor bounding box not found");
    await page.mouse.click(box.x + 20, box.y + box.height / 2);
    await page.waitForTimeout(200);
    await page.keyboard.type(text);
    await page.waitForTimeout(100);
    const content = await editor.textContent();
    if (content?.includes(text)) return;
    // Text wasn't entered; select all and clear for retry
    await page.keyboard.press(`${modifier}+a`);
    await page.keyboard.press("Backspace");
    await page.waitForTimeout(200);
  }
  throw new Error(`Failed to type "${text}" into editor after 3 attempts`);
}

// Allow 1 retry: the test can be flaky when a previous test cycle's agent process hasn't
// fully shut down, causing the new session to conflict with a stale execution.
test.describe.configure({ retries: 1 });

test("quick chat queued message indicator appears and message executes after agent turn", async ({
  testPage,
}) => {
  test.setTimeout(60_000);

  const dialog = await openQuickChatWithAgent(testPage);

  // Send a slow command so the agent stays busy for 10 seconds.
  const editor = dialog.locator(".tiptap.ProseMirror");
  await editor.click();
  await editor.fill("/slow 10s");
  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await editor.press(`${modifier}+Enter`);

  // Wait for agent to become busy.
  await expect(testPage.getByRole("status", { name: /Agent is (starting|running)/ })).toBeVisible({
    timeout: 15_000,
  });
  await testPage.waitForTimeout(500);

  await typeWhileBusy(testPage, editor, "hello world");
  await testPage.keyboard.press(`${modifier}+Enter`);

  // Verify the queued message indicator with cancel button appears.
  const cancelBtn = dialog.getByTitle("Cancel queued message");
  await expect(cancelBtn).toBeVisible({ timeout: 10_000 });

  // Wait for the first (slow) response to complete.
  await expect(dialog.getByText("Slow response complete", { exact: false })).toBeVisible({
    timeout: 30_000,
  });

  // The queued message should auto-execute — wait for the agent turn to finish.
  // The idle placeholder confirms the agent completed processing the queued message.
  await expect(dialog.locator('[data-placeholder="Continue working on the task..."]')).toBeVisible({
    timeout: 30_000,
  });
});

test("quick chat queue message via submit button click", async ({ testPage }) => {
  test.setTimeout(90_000);

  const dialog = await openQuickChatWithAgent(testPage);

  // Send a slow command so the agent stays busy for 10 seconds.
  const editor = dialog.locator(".tiptap.ProseMirror");
  await editor.click();
  await editor.fill("/slow 10s");
  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await editor.press(`${modifier}+Enter`);

  // Wait for agent to become busy.
  await expect(testPage.getByRole("status", { name: /Agent is (starting|running)/ })).toBeVisible({
    timeout: 15_000,
  });
  await testPage.waitForTimeout(500);

  // Before typing, only the cancel button should be visible (no send button).
  const submitBtn = dialog.getByTestId("submit-message-button");
  await expect(submitBtn).not.toBeVisible();
  await expect(dialog.getByTestId("cancel-agent-button")).toBeVisible();

  // Type a queued message — the submit button should appear.
  await typeWhileBusy(testPage, editor, "queued via button");
  await expect(submitBtn).toBeVisible({ timeout: 5_000 });

  // Click the submit button (not keyboard shortcut) to queue the message.
  await submitBtn.click();

  // Verify the queued message indicator with cancel button appears.
  const cancelBtn = dialog.getByTitle("Cancel queued message");
  await expect(cancelBtn).toBeVisible({ timeout: 10_000 });

  // Verify the cancel-agent button is also visible alongside submit.
  const cancelAgentBtn = dialog.getByTestId("cancel-agent-button");
  await expect(cancelAgentBtn).toBeVisible();

  // Wait for the first (slow) response to complete and queued message to auto-execute.
  // Allow extra time: 10s slow + agent turn overhead + queue execution.
  await expect(dialog.locator('[data-placeholder="Continue working on the task..."]')).toBeVisible({
    timeout: 60_000,
  });
});
