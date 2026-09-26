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
 * The cancel button is also available while direct input is working, so it is
 * not a queue-mode signal. Wait on the explicit queue-derived input mode,
 * scoped to the visible composer in the supplied surface.
 */
export async function waitForComposerQueueMode(
  scope: Page | Locator,
  timeout = 15_000,
): Promise<void> {
  const activeComposer = scope.locator('[data-testid="chat-input-area"]:visible');
  await expect(activeComposer).toHaveAttribute("data-input-mode", "queue", { timeout });
}

/**
 * Type text into the TipTap editor while the agent is busy.
 * fill() silently fails on TipTap when the busy placeholder is shown,
 * so we retry clicking and typing until text appears in the editor.
 */
export async function typeWhileBusy(page: Page, editor: Locator, text: string): Promise<void> {
  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  const coarsePointer = await page.evaluate(() => window.matchMedia("(pointer: coarse)").matches);
  await editor.scrollIntoViewIfNeeded();
  for (let attempt = 0; attempt < 3; attempt++) {
    const box = await editor.boundingBox();
    if (!box) throw new Error("Editor bounding box not found");
    if (coarsePointer) {
      await editor.tap({ position: { x: 20, y: box.height / 2 } });
    } else {
      await page.mouse.click(box.x + 20, box.y + box.height / 2);
    }
    const focused = await expect(editor)
      .toBeFocused({ timeout: 5_000 })
      .then(() => true)
      .catch(() => false);
    if (!focused) continue;
    await page.keyboard.type(text);
    // Auto-retrying, so it returns as soon as ProseMirror renders the text
    // rather than after a fixed settle. A miss here is the retry's cue, not a
    // failure, hence the catch.
    const typed = await expect(editor)
      .toContainText(text, { timeout: 2_000 })
      .then(() => true)
      .catch(() => false);
    if (typed) return;
    // Text wasn't entered; select all and clear for retry. Scoped to the editor
    // rather than sent through `page.keyboard`, because this branch runs
    // precisely when the typing may have gone somewhere else -- a page-level
    // select-all + Backspace is aimed at whatever holds focus, which is the one
    // thing this path cannot assume. `locator.press` focuses first.
    await editor.press(`${modifier}+a`);
    await editor.press("Backspace");
    // Wait for the clear to land instead of guessing, and require the editor to
    // be *empty*: asserting only that `text` is gone would let leftovers from a
    // partial attempt survive and concatenate with the next one, which the
    // following `toContainText` would then happily accept.
    await expect
      .poll(async () => (await editor.innerText()).trim(), {
        timeout: 5_000,
        message: "editor still had content after select-all + Backspace",
      })
      .toBe("");
  }
  throw new Error(`Failed to type "${text}" into editor after 3 attempts`);
}
