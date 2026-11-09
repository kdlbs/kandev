import { test, expect } from "../../fixtures/test-base";
import {
  expectNativeCompactionCount,
  seedCopilotNativeTask,
} from "./copilot-native-compaction-helpers";

test.describe("Copilot native compaction on phone", () => {
  test.setTimeout(120_000);

  test("shows the same count and status without horizontal overflow", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const release = await backend.useEnv({
      KANDEV_E2E_MOCK_COPILOT: "true",
      KANDEV_MOCK_COPILOT_NATIVE: "true",
    });
    try {
      await seedCopilotNativeTask(testPage, apiClient, seedData, "/e2e:copilot-native-partial");

      const trigger = testPage.getByRole("button", { name: /Context window: / });
      await trigger.tap();
      const tooltip = testPage
        .locator('[data-slot="tooltip-content"]:not([data-state="closed"])')
        .filter({ has: testPage.getByTestId("context-window-usage") });
      await expect(tooltip.getByTestId("context-window-usage").first()).toBeVisible();
      const usage = tooltip.getByTestId("context-window-usage").first();
      await expectNativeCompactionCount(tooltip, 1);
      await expect(usage.getByTestId("context-window-compactions-status").first()).toHaveText(
        "Partial",
      );

      const hasHorizontalOverflow = await testPage.evaluate(() => {
        const root = document.scrollingElement ?? document.documentElement;
        return root.scrollWidth > root.clientWidth + 1;
      });
      expect(hasHorizontalOverflow).toBe(false);

      const helpButton = usage
        .locator('button[aria-label="About inferred compaction count"]')
        .first();
      await helpButton.tap();
      const helpId = await helpButton.getAttribute("aria-describedby");
      if (!helpId) throw new Error("Expected compaction help to be described");
      await expect(testPage.locator(`[id="${helpId}"]`)).toHaveCSS("opacity", "1");

      await prCapture.screenshot("mobile-copilot-native-compaction", {
        caption: "Phone compaction count and partial coverage help",
      });
    } finally {
      await release();
    }
  });
});
