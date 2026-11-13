import type { Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { closeQuickTerminalTab } from "./terminal-test-helpers";
import {
  captureQuickTerminalRenderRequests,
  exerciseQuickTerminalRenderContinuity,
  prepareQuickTerminalForCapture,
  settleQuickTerminalAnimationsForCapture,
} from "./quick-terminal-render-continuity-helpers";

async function closeSurvivingQuickTerminals(testPage: Page) {
  const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
  if (!(await dialog.isVisible().catch(() => false))) {
    const launcher = testPage.getByTestId("sidebar-quick-terminal-shortcut");
    if (await launcher.isVisible().catch(() => false)) await launcher.click();
  }
  if (!(await dialog.isVisible().catch(() => false))) return;

  const tabs = dialog.getByTestId("quick-terminal-tab");
  for (let attempts = 0; attempts < 8; attempts += 1) {
    const count = await tabs.count();
    if (count === 0) break;
    await closeQuickTerminalTab(testPage, tabs.nth(count - 1));
    await expect(tabs).toHaveCount(count - 1, { timeout: 10_000 });
  }
  await expect(tabs).toHaveCount(0);
}

test.describe("Quick Terminal render continuity", () => {
  test("keeps command input and the original stream through unrelated renders", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    const { settings } = await apiClient.getUserSettings();
    const previousSidebarSettings = {
      sidebar_fast_actions_enabled: settings.sidebar_fast_actions_enabled ?? false,
      sidebar_new_task_style: settings.sidebar_new_task_style ?? "simple",
    };
    const capture = captureQuickTerminalRenderRequests(testPage);

    try {
      await apiClient.saveUserSettings({
        sidebar_fast_actions_enabled: true,
        sidebar_new_task_style: "compact",
      });
      await testPage.goto("/");

      const launcher = testPage.getByTestId("sidebar-quick-terminal-shortcut");
      await expect(launcher).toBeVisible();
      const terminalInput = {
        focus: async () => testPage.getByTestId("quick-terminal-terminal").click(),
        type: async (text: string) => testPage.keyboard.type(text),
      };
      await exerciseQuickTerminalRenderContinuity(testPage, capture, terminalInput, async () =>
        launcher.click(),
      );
      if (prCapture.capturing) {
        const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
        await expect(dialog).toBeVisible();
        await expect(testPage.getByText("Kandev update available", { exact: true })).toBeHidden({
          timeout: 10_000,
        });
        await prepareQuickTerminalForCapture(testPage, terminalInput);
        await settleQuickTerminalAnimationsForCapture(testPage);
        await prCapture.screenshot("desktop", {
          caption: "Desktop Quick Terminal retains output through unrelated renders",
        });
      }
    } finally {
      try {
        await closeSurvivingQuickTerminals(testPage);
      } finally {
        await apiClient.saveUserSettings(previousSidebarSettings);
      }
    }
  });
});
