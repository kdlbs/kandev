import { test, expect } from "../../fixtures/test-base";
import {
  expectSourceRightOfTokenCount,
  seedContextWindowTask,
} from "./context-window-source-helpers";

test("context source help is reachable by touch without overflow", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await seedContextWindowTask(testPage, apiClient, seedData);

  const contextTrigger = testPage.getByRole("button", { name: "Context window: 21% used" });
  await contextTrigger.tap();
  const contextTooltip = testPage
    .locator('[data-slot="tooltip-content"][data-state]')
    .filter({ has: testPage.getByTestId("context-window-usage") });
  const contextUsage = contextTooltip.getByTestId("context-window-usage").first();
  await expect(contextUsage).toBeVisible();
  await expectSourceRightOfTokenCount(contextTooltip);

  const sourceHelpButton = contextUsage.locator('button[aria-label="About context window source"]');
  const sourceHelpId = await sourceHelpButton.getAttribute("aria-describedby");
  if (!sourceHelpId) throw new Error("Expected source help to be described");
  await sourceHelpButton.tap();

  await expect(contextUsage).toBeVisible();
  await expect(testPage.locator(`[id="${sourceHelpId}"]`)).toHaveCSS("opacity", "1");
  const hasHorizontalOverflow = await testPage.evaluate(() => {
    const root = document.scrollingElement ?? document.documentElement;
    return root.scrollWidth > root.clientWidth + 1;
  });
  expect(hasHorizontalOverflow).toBe(false);
});
