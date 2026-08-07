import { test, expect } from "../../fixtures/test-base";
import {
  expectCompactionCount,
  expectSourceRightOfTokenCount,
  seedContextWindowTask,
} from "./context-window-source-helpers";

test("pending context usage is reachable by touch without overflow", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await seedContextWindowTask(testPage, apiClient, seedData, {
    used: 0,
    remaining: 258_400,
    efficiency: 0,
  });

  const contextTrigger = testPage.getByRole("button", {
    name: "Context window: usage not measured",
  });
  await contextTrigger.tap();
  const contextTooltip = testPage
    .locator('[data-slot="tooltip-content"]:not([data-state="closed"])')
    .filter({ has: testPage.getByTestId("context-window-usage") });
  // Radix renders a visually-hidden accessibility copy of the content with the
  // same testid; scope to the visible instance like the sibling positive test.
  const contextUsage = contextTooltip.getByTestId("context-window-usage").first();
  await expect(contextUsage).toBeVisible();
  await expect(contextUsage.getByText("—%", { exact: true })).toBeVisible();
  await expect(contextUsage.getByText("— of 258.4K tokens", { exact: true })).toBeVisible();
  await expect(
    contextUsage.getByText(/Usage data appears after the first completed turn/),
  ).toBeVisible();
  await expect(contextUsage.getByText("0%", { exact: true })).toHaveCount(0);
  await expect(contextUsage.getByText(/0 of/)).toHaveCount(0);
  await expectSourceRightOfTokenCount(contextTooltip);
  await expectCompactionCount(contextTooltip);

  const hasHorizontalOverflow = await testPage.evaluate(() => {
    const root = document.scrollingElement ?? document.documentElement;
    return root.scrollWidth > root.clientWidth + 1;
  });
  expect(hasHorizontalOverflow).toBe(false);
});

test("context source help is reachable by touch without overflow", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await seedContextWindowTask(testPage, apiClient, seedData);

  const contextTrigger = testPage.getByRole("button", { name: "Context window: 21% used" });
  await contextTrigger.tap();
  const contextTooltip = testPage
    .locator('[data-slot="tooltip-content"]:not([data-state="closed"])')
    .filter({ has: testPage.getByTestId("context-window-usage") });
  const contextUsage = contextTooltip.getByTestId("context-window-usage").first();
  await expect(contextUsage).toBeVisible();
  await expectSourceRightOfTokenCount(contextTooltip);
  await expectCompactionCount(contextTooltip);

  const sourceHelpButton = contextUsage.locator('button[aria-label="About context window source"]');
  const sourceHelpId = await sourceHelpButton.getAttribute("aria-describedby");
  if (!sourceHelpId) throw new Error("Expected source help to be described");
  await sourceHelpButton.tap();

  await expect(contextUsage).toBeVisible();
  await expect(testPage.locator(`[id="${sourceHelpId}"]`)).toHaveCSS("opacity", "1");

  const compactionHelpButton = contextUsage.locator(
    'button[aria-label="About inferred compaction count"]',
  );
  const compactionHelpId = await compactionHelpButton.getAttribute("aria-describedby");
  if (!compactionHelpId) throw new Error("Expected compaction help to be described");
  await compactionHelpButton.tap();

  await expect(contextUsage).toBeVisible();
  await expect(testPage.locator(`[id="${compactionHelpId}"]`)).toHaveCSS("opacity", "1");
  await compactionHelpButton.tap();
  await expect(testPage.locator(`[id="${compactionHelpId}"]`)).toHaveCSS("opacity", "0");
  const hasHorizontalOverflow = await testPage.evaluate(() => {
    const root = document.scrollingElement ?? document.documentElement;
    return root.scrollWidth > root.clientWidth + 1;
  });
  expect(hasHorizontalOverflow).toBe(false);
});
