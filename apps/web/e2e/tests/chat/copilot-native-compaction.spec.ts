import { test, expect } from "../../fixtures/test-base";
import {
  ensureContextTooltipOpen,
  expectNativeCompactionCount,
  openContextTooltip,
  seedCopilotNativeTask,
  sendCopilotLiveCompaction,
} from "./copilot-native-compaction-helpers";

test.describe("Copilot native compaction", () => {
  test.setTimeout(120_000);

  test("counts one applied parent completion and ignores child and duplicate frames", async ({
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
      await seedCopilotNativeTask(testPage, apiClient, seedData, "/e2e:copilot-native");

      const { tooltip } = await openContextTooltip(testPage);
      await expectNativeCompactionCount(tooltip, 1);
      await expect(tooltip.getByTestId("context-window-compactions-status")).toHaveCount(0);
      const reopened = await ensureContextTooltipOpen(testPage);
      const helpButton = reopened.tooltip
        .locator('button[aria-label="About inferred compaction count"]')
        .first();
      await helpButton.hover();
      const helpId = await helpButton.getAttribute("aria-describedby");
      if (!helpId) throw new Error("Expected compaction help to be described");
      const help = testPage.locator(`[id="${helpId}"]`);
      await expect(help).toHaveCSS("opacity", "1");
      await expect(help).toContainText("successful parent compaction events");
      await expect(help).toContainText("Historical estimates remain");

      // The gauge comes from root usage after the applied compaction.
      await expect(testPage.getByRole("button", { name: "Context window: 4% used" })).toBeVisible();
      const gauge = await ensureContextTooltipOpen(testPage);
      await expect(gauge.tooltip.getByTestId("context-window-usage").first()).toContainText(
        "9.7K of 272.0K",
      );

      // A distinct completion after page load proves live publication, not
      // only the hydrated snapshot from seedCopilotNativeTask.
      await sendCopilotLiveCompaction(testPage);
      const live = await openContextTooltip(testPage);
      await expectNativeCompactionCount(live.tooltip, 2);
      await expect(testPage.getByRole("button", { name: "Context window: 3% used" })).toBeVisible();

      await backend.restart();
      await testPage.reload();
      const afterRestart = await openContextTooltip(testPage);
      await expectNativeCompactionCount(afterRestart.tooltip, 2);
      await expect(testPage.getByRole("button", { name: "Context window: 3% used" })).toBeVisible();

      await prCapture.screenshot("copilot-native-compaction", {
        caption: "Copilot native compaction count with retained root context gauge",
      });
    } finally {
      await release();
    }
  });

  test("keeps partial coverage after omitted completion evidence", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const release = await backend.useEnv({
      KANDEV_E2E_MOCK_COPILOT: "true",
      KANDEV_MOCK_COPILOT_NATIVE: "true",
    });
    try {
      await seedCopilotNativeTask(testPage, apiClient, seedData, "/e2e:copilot-native-partial");

      const { tooltip } = await openContextTooltip(testPage);
      await expectNativeCompactionCount(tooltip, 1);
      await expect(tooltip.getByTestId("context-window-compactions-status").first()).toHaveText(
        "Partial",
      );
      const reopened = await ensureContextTooltipOpen(testPage);
      await expect(reopened.tooltip.getByTestId("context-window-usage").first()).toContainText(
        "Some compaction evidence is missing",
      );
    } finally {
      await release();
    }
  });
});
