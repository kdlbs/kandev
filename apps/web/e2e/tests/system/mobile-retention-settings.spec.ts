import { test, expect } from "../../fixtures/test-base";

test.describe("Office retention settings on phones", () => {
  test("keeps policy fields in one column with touch-sized controls", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto("/settings/system/storage?tab=office-retention");

    await expect(testPage.getByTestId("retention-policy-card")).toBeVisible();
    const visibleFields = [
      "retention-routine-runs-window-days",
      "retention-routine-runs-floor-per-owner",
      "retention-runs-window-days",
      "retention-runs-floor-per-owner",
    ];
    for (const id of visibleFields) {
      const field = testPage.getByTestId(id);
      const box = await field.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height, id).toBeGreaterThanOrEqual(44);
    }

    const advanced = testPage.getByTestId("retention-advanced-settings");
    await expect(advanced).not.toHaveAttribute("open");
    await advanced.locator("summary").tap();
    await expect(advanced).toHaveAttribute("open");
    for (const id of [
      "retention-sweep-interval",
      "retention-batch-limit",
      "retention-run-events-warn-rows",
    ]) {
      const field = testPage.getByTestId(id);
      const box = await field.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height, id).toBeGreaterThanOrEqual(44);
    }
    await advanced.locator("summary").tap();
    await expect(advanced).not.toHaveAttribute("open");

    await expect
      .poll(() =>
        testPage.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      )
      .toBe(true);
  });
});
