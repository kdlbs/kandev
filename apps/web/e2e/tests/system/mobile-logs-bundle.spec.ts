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

  test("opens the customizer in an inset drawer with bounded source choices", async ({
    testPage,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto("/settings/system/logs");

    const customize = testPage.getByTestId("customize-diagnostic-bundle");
    await expect(customize).toBeVisible();
    expect((await customize.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
    await customize.tap();

    const drawer = testPage.getByTestId("diagnostic-bundle-drawer");
    await expect(drawer).toBeVisible();
    await expect(drawer.getByRole("heading", { name: "Evidence sources" })).toBeVisible();
    const create = drawer.getByTestId("create-custom-diagnostic-bundle");
    expect((await create.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
      ),
    ).toBe(false);
  });
});
