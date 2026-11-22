import { expect, test } from "../../fixtures/test-base";
import { closeQuickTerminalTab } from "./terminal-test-helpers";
import {
  captureQuickTerminalRenderRequests,
  exerciseQuickTerminalRenderContinuity,
  prepareQuickTerminalForCapture,
  settleQuickTerminalAnimationsForCapture,
} from "./quick-terminal-render-continuity-helpers";

test.describe("Mobile Quick Terminal render continuity", () => {
  test("keeps command input and phone controls through unrelated renders", async ({
    testPage,
    prCapture,
  }, testInfo) => {
    test.setTimeout(120_000);
    const capture = captureQuickTerminalRenderRequests(testPage);
    const terminalInput = {
      focus: async () => testPage.getByTestId("quick-terminal-terminal").tap(),
      type: async (text: string) => testPage.keyboard.type(text),
    };
    await testPage.goto("/");

    try {
      await exerciseQuickTerminalRenderContinuity(testPage, capture, terminalInput, async () => {
        await testPage.getByTestId("app-nav-trigger").tap();
        await testPage.getByTestId("mobile-quick-terminal-button").tap();
      });

      const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
      const keybar = dialog.getByTestId("mobile-terminal-keybar");
      await expect(keybar).toBeVisible();
      for (const key of ["ctrl", "ctrl-c", "tab"]) {
        const box = await keybar.getByTestId(`keybar-key-${key}`).boundingBox();
        expect(box).not.toBeNull();
        expect(box!.width).toBeGreaterThanOrEqual(44);
        expect(box!.height).toBeGreaterThanOrEqual(44);
      }
      const [keybarBox, viewport] = await Promise.all([
        keybar.boundingBox(),
        testPage.evaluate(() => ({
          top: window.visualViewport?.offsetTop ?? 0,
          height: window.visualViewport?.height ?? window.innerHeight,
        })),
      ]);
      expect(keybarBox).not.toBeNull();
      expect(keybarBox!.y + keybarBox!.height).toBeLessThanOrEqual(
        viewport.top + viewport.height + 1,
      );
      await expect(dialog.locator(".xterm-helper-textarea")).toBeFocused();
      await expect(testPage.getByText("Kandev update available", { exact: true })).toBeHidden({
        timeout: 10_000,
      });
      if (prCapture.capturing) {
        await prepareQuickTerminalForCapture(testPage, terminalInput);
        await settleQuickTerminalAnimationsForCapture(testPage);
      }
      await testPage.screenshot({
        path: testInfo.outputPath("quick-terminal-render-continuity.png"),
      });
      await prCapture.screenshot("quick-terminal-render-continuity", {
        caption:
          "Phone Quick Terminal retains input and shortcut controls across unrelated renders",
      });
    } finally {
      const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
      if (!(await dialog.isVisible().catch(() => false))) {
        const menu = testPage.getByTestId("app-nav-trigger");
        if (await menu.isVisible().catch(() => false)) {
          if ((await menu.getAttribute("aria-expanded")) !== "true") await menu.tap();
          await testPage.getByTestId("mobile-quick-terminal-button").tap();
        }
      }
      if (await dialog.isVisible().catch(() => false)) {
        const tabs = dialog.getByTestId("quick-terminal-tab");
        for (let attempts = 0; attempts < 8; attempts += 1) {
          const count = await tabs.count();
          if (count === 0) break;
          await closeQuickTerminalTab(testPage, tabs.nth(count - 1));
          await expect(tabs).toHaveCount(count - 1, { timeout: 10_000 });
        }
        await expect(tabs).toHaveCount(0);
      }
    }
  });
});
