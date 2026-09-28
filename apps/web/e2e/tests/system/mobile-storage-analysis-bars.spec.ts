import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { mockStorageAnalysisBarsOverview } from "../../helpers/storage-maintenance";

async function assertNoHorizontalOverflow(page: Page) {
  await expect
    .poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth))
    .toBe(true);
}

test.describe("Mobile storage analysis bars", () => {
  test("keeps bars below the mobile header and rows touch-sized", async ({
    testPage,
    prCapture,
  }) => {
    await mockStorageAnalysisBarsOverview(testPage);
    await testPage.goto("/settings/system/storage");

    const workspaceTrigger = testPage.getByTestId("storage-resource-workspaces-trigger");
    const systemTrigger = testPage.getByTestId("storage-resource-system-temporary-trigger");
    for (const trigger of [workspaceTrigger, systemTrigger]) {
      const box = await trigger.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
    }

    const workspaceLabel = testPage.getByText("Task workspaces", { exact: true });
    const workspaceBar = testPage.getByTestId("storage-resource-workspaces-bar");
    const labelBox = await workspaceLabel.boundingBox();
    const barBox = await workspaceBar.boundingBox();
    expect(labelBox).not.toBeNull();
    expect(barBox).not.toBeNull();
    expect(barBox!.y).toBeGreaterThanOrEqual(labelBox!.y + labelBox!.height - 1);
    expect(barBox!.width).toBeGreaterThan(0);

    await systemTrigger.tap();
    const resource = testPage.getByTestId("storage-resource-system-temporary");
    await expect(resource).toContainText("Partial");
    await expect(resource).toContainText("Scan timed out. Showing partial usage.");
    await assertNoHorizontalOverflow(testPage);
    await prCapture.screenshot("mobile-storage-analysis-bars", {
      caption: "Phone storage analysis places the relative bar below the category header",
      fullPage: true,
    });
  });

  for (const width of [767, 768]) {
    test(`keeps the storage header contained at ${width}px`, async ({ testPage }) => {
      await testPage.setViewportSize({ width, height: 900 });
      await mockStorageAnalysisBarsOverview(testPage);
      await testPage.goto("/settings/system/storage");

      const trigger = testPage.getByTestId("storage-resource-workspaces-trigger");
      const bar = testPage.getByTestId("storage-resource-workspaces-bar");
      const triggerBox = await trigger.boundingBox();
      const barBox = await bar.boundingBox();
      expect(triggerBox).not.toBeNull();
      expect(barBox).not.toBeNull();
      expect(barBox!.x).toBeGreaterThanOrEqual(triggerBox!.x);
      expect(barBox!.x + barBox!.width).toBeLessThanOrEqual(triggerBox!.x + triggerBox!.width + 1);
      await assertNoHorizontalOverflow(testPage);
    });
  }
});
