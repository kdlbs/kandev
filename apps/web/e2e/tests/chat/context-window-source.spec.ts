import { test, expect } from "../../fixtures/test-base";
import {
  expectSourceRightOfTokenCount,
  seedContextWindowTask,
} from "./context-window-source-helpers";

test("context source help stays open when hovered", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  await seedContextWindowTask(testPage, apiClient, seedData);

  const contextTrigger = testPage.getByRole("button", { name: "Context window: 21% used" });
  await contextTrigger.hover();
  const contextTooltip = testPage
    .locator('[data-slot="tooltip-content"][data-state]')
    .filter({ has: testPage.getByTestId("context-window-usage") });
  const contextUsage = contextTooltip.getByTestId("context-window-usage").first();
  await expect(contextUsage).toBeVisible();
  await expectSourceRightOfTokenCount(contextTooltip);

  const sourceHelpButton = contextUsage.locator('button[aria-label="About context window source"]');
  const sourceHelpId = await sourceHelpButton.getAttribute("aria-describedby");
  if (!sourceHelpId) throw new Error("Expected source help to be described");
  await sourceHelpButton.hover();

  await expect(contextUsage).toBeVisible();
  await expect(testPage.locator(`[id="${sourceHelpId}"]`)).toHaveCSS("opacity", "1");
  await prCapture.screenshot("context-source-help", {
    caption: "Context source shown inline with the token count and its help visible",
  });
});
