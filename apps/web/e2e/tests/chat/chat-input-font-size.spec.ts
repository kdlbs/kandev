import { test, expect } from "../../fixtures/test-base";
import { composerEditor, createReadyTask, openTaskChat } from "./chat-input-font-size-helpers";

/** Widths on both sides of Tailwind's `lg` (1024px) breakpoint. The composer
 *  used to carry `text-base … lg:text-sm`, so dragging a desktop window under
 *  1024px grew the prompt text from 14px to 16px. */
const WIDTHS = [1440, 1100, 900, 700] as const;

/** Typed into the composer purely so the screenshots show real glyphs to
 *  compare across widths. */
const SAMPLE_PROMPT = "The quick brown fox jumps over the lazy dog";

test.describe("Chat input font size", () => {
  test("stays constant while the desktop window is resized", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const task = await createReadyTask(apiClient, seedData, "Chat Input Font Size");
    const session = await openTaskChat(testPage, task.id);

    const editor = composerEditor(session);
    await expect(editor).toBeVisible();

    // Desktop Chrome exposes a fine pointer, so the globals.css 16px
    // coarse-pointer floor must not apply here. Assert it, otherwise this test
    // would pass trivially with every width pinned to 16px.
    const coarse = await testPage.evaluate(() => matchMedia("(any-pointer: coarse)").matches);
    expect(coarse).toBe(false);

    const sizes: number[] = [];
    for (const width of WIDTHS) {
      await testPage.setViewportSize({ width, height: 900 });
      // Narrow widths swap the dockview layout, which remounts the composer.
      // Re-await visibility so the measurement never lands on a detached node
      // mid-remount (getComputedStyle on a detached element yields "" → NaN).
      await expect(editor).toBeVisible();
      await expect
        .poll(async () => editor.evaluate((el) => getComputedStyle(el).fontSize))
        .not.toBe("");
      sizes.push(await editor.evaluate((el) => parseFloat(getComputedStyle(el).fontSize)));
    }

    expect(sizes).toEqual(sizes.map(() => sizes[0]));
    expect(sizes[0]).toBeCloseTo(14, 1);

    // Capture the wide/narrow pair for the PR description (only when
    // CAPTURE_PR_ASSETS=true). The same prompt text is typed once and shot at
    // both widths so the two images can be compared directly.
    await testPage.setViewportSize({ width: 1440, height: 900 });
    await expect(editor).toBeVisible();
    await editor.click();
    await editor.pressSequentially(SAMPLE_PROMPT);
    await expect(editor).toContainText(SAMPLE_PROMPT);
    await prCapture.screenshot("composer-1440px", {
      caption: "Composer at 1440px (above the lg breakpoint) — prompt text at 14px",
    });

    await testPage.setViewportSize({ width: 900, height: 900 });
    await expect(editor).toBeVisible();
    await expect(editor).toContainText(SAMPLE_PROMPT);
    await prCapture.screenshot("composer-900px", {
      caption: "Composer at 900px (below the lg breakpoint) — prompt text still 14px, was 16px",
    });
  });
});
