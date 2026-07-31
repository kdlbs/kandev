import { test, expect } from "../../fixtures/test-base";
import { composerEditor, createReadyTask, openTaskChat } from "./chat-input-font-size-helpers";

// Runs on the mobile-chrome Playwright project (Pixel 5 emulation, touch →
// any-pointer: coarse). The composer's Tailwind class is a flat `text-sm`, so
// touch devices depend entirely on the 16px coarse-pointer rule in globals.css
// to stop iOS Safari auto-zooming on focus. This guards that dependency —
// mobile-zoom.spec.ts covers plain form controls, this covers the
// contenteditable TipTap editor.
test.describe("Mobile chat input font size", () => {
  test("composer renders at >= 16px to prevent iOS focus-zoom", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const task = await createReadyTask(apiClient, seedData, "Mobile Chat Input Font Size");
    await openTaskChat(testPage, task.id);

    // Guard: the 16px rule is gated on `@media (any-pointer: coarse)`. Assert
    // the emulated device really exposes a coarse pointer so a fixture change
    // that drops touch emulation fails loudly instead of silently passing.
    const coarse = await testPage.evaluate(() => matchMedia("(any-pointer: coarse)").matches);
    expect(coarse).toBe(true);

    const editor = composerEditor(testPage);
    await expect(editor).toBeVisible();
    await editor.tap();

    const fontSize = await editor.evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
    expect(fontSize).toBeGreaterThanOrEqual(16);

    await editor.pressSequentially("The quick brown fox jumps over the lazy dog");
    await prCapture.screenshot("mobile-composer", {
      caption: "Touch composer keeps the 16px iOS focus-zoom floor from the coarse-pointer rule",
    });
  });
});
