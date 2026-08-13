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
    // Typing requires the editor to own focus, and `mouse.click` only queues
    // that: ProseMirror commits focus on a later tick. This waits for the
    // commit rather than for a number.
    //
    // It replaces an `unverified` 200ms sleep. The hypothesis for that sleep
    // was that keystrokes sent before the focus commit go to the previously
    // focused element -- but that is NOT confirmed: with the sleep removed and
    // nothing in its place, 9/9 calls still landed their text on the first
    // attempt locally. So this is a precondition guard, not a fix for an
    // observed failure. It is kept because it is free when focus is already
    // committed and correct if a loaded shard ever does reorder the two, which
    // a 9-call sample on an idle machine cannot rule out.
    await expect(editor).toBeFocused({ timeout: 5_000 });
    await page.keyboard.type(text);
    // Auto-retrying, so it returns as soon as ProseMirror renders the text
    // rather than after a fixed settle. A miss here is the retry's cue, not a
    // failure, hence the catch.
    const typed = await expect(editor)
      .toContainText(text, { timeout: 2_000 })
      .then(() => true)
      .catch(() => false);
    if (typed) return;
    // Text wasn't entered; select all and clear for retry.
    await page.keyboard.press(`${modifier}+a`);
    await page.keyboard.press("Backspace");
    // Wait for the clear to land instead of guessing: the next attempt's
    // `toContainText` would otherwise match leftovers from this one.
    await expect(editor).not.toContainText(text, { timeout: 5_000 });
  }
  throw new Error(`Failed to type "${text}" into editor after 3 attempts`);
}
