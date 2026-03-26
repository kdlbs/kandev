import { type Locator, type Page } from "@playwright/test";
import { test, expect } from "../fixtures/test-base";

/**
 * Quick Chat queued message E2E test.
 *
 * Isolated into its own file because the agent stays busy for 10s during the test,
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

  // Type a queued message: fill() silently fails on TipTap when the busy placeholder
  // is shown. Retry clicking and typing until text appears in the editor.
  await editor.scrollIntoViewIfNeeded();
  for (let attempt = 0; attempt < 3; attempt++) {
    const box = await editor.boundingBox();
    if (!box) throw new Error("Editor bounding box not found");
    await testPage.mouse.click(box.x + 20, box.y + box.height / 2);
    await testPage.waitForTimeout(200);
    await testPage.keyboard.type("hello world");
    await testPage.waitForTimeout(100);
    const text = await editor.textContent();
    if (text?.includes("hello world")) break;
    // Text wasn't entered; select all and clear for retry
    await testPage.keyboard.press(`${modifier}+a`);
    await testPage.keyboard.press("Backspace");
    await testPage.waitForTimeout(200);
  }
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
