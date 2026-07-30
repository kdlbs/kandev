import { test, expect } from "../../fixtures/test-base";

test.describe("System Logs mobile", () => {
  test("keeps the combined download action touch-accessible without horizontal overflow", async ({
    testPage,
    prCapture,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto("/settings/system/logs");

    const action = testPage.getByTestId("download-diagnostic-bundle");
    await expect(action).toBeVisible();
    expect((await action.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
    const overflow = await testPage.evaluate(
      () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
    );
    expect(overflow).toBe(false);
    await prCapture.screenshot("mobile-combined-diagnostic-logs", {
      caption: "The combined diagnostic download remains touch-accessible on mobile.",
      fullPage: true,
    });
  });
});
