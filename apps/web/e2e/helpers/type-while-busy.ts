import { expect, type Locator, type Page } from "@playwright/test";

/**
 * Wait until the composer has actually entered queue ("busy") mode.
 *
 * The `Agent is starting|running` status indicator renders straight off
 * `session.state`, but the composer's queue affordance comes from a separate
 * derivation (`deriveSessionInputMode`: state + foreground_activity +
 * supports_steering). Asserting the status alone can therefore return before
 * the toolbar has re-rendered into queue mode, which is why these call sites
 * used to chase it with a fixed sleep.
 *
 * The cancel button is the rendered signal of that second derivation, so
 * waiting on it gives the same guarantee reactively: it returns immediately
 * when the composer is already busy and keeps waiting when the store is slow,
 * instead of spending a fixed budget that is simultaneously too long and too
 * short.
 */
export async function waitForComposerQueueMode(
  scope: Page | Locator,
  timeout = 15_000,
): Promise<void> {
  await expect(scope.getByTestId("cancel-agent-button")).toBeVisible({ timeout });
}

/**
 * Type text into the TipTap editor while the agent is busy.
 * fill() silently fails on TipTap when the busy placeholder is shown,
 * so we retry clicking and typing until text appears in the editor.
 */
export async function typeWhileBusy(page: Page, editor: Locator, text: string): Promise<void> {
  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await editor.scrollIntoViewIfNeeded();
  for (let attempt = 0; attempt < 3; attempt++) {
    const box = await editor.boundingBox();
    if (!box) throw new Error("Editor bounding box not found");
    await page.mouse.click(box.x + 20, box.y + box.height / 2);
    // deliberate-sleep(unverified): pre-existing spacing in this retry loop.
    // Unlike the other retained sleeps these are not tied to an identified
    // timer, so they are debt rather than intent: the likely reactive
    // replacement is waiting for the editor to take focus. They are left alone
    // here only because this helper is on the hot path of ~16 specs that were
    // just verified against it, and changing it would invalidate that run.
    await page.waitForTimeout(200);
    await page.keyboard.type(text);
    // deliberate-sleep(unverified): as above.
    await page.waitForTimeout(100);
    const content = await editor.textContent();
    if (content?.includes(text)) return;
    // Text wasn't entered; select all and clear for retry
    await page.keyboard.press(`${modifier}+a`);
    await page.keyboard.press("Backspace");
    // deliberate-sleep(unverified): as above.
    await page.waitForTimeout(200);
  }
  throw new Error(`Failed to type "${text}" into editor after 3 attempts`);
}
