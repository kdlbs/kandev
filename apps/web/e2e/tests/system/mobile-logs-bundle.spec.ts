import { test, expect } from "../../fixtures/test-base";

test.describe("System Logs mobile", () => {
  test("keeps the one customizer action touch-accessible without horizontal overflow", async ({
    testPage,
    prCapture,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto("/settings/system/logs");

    const action = testPage.getByTestId("customize-diagnostic-bundle");
    await expect(action).toBeVisible();
    expect((await action.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
    await action.tap();
    const drawer = testPage.getByTestId("diagnostic-bundle-drawer");
    await expect(drawer).toBeVisible();
    await expect(testPage.getByTestId("download-diagnostic-bundle")).toHaveCount(0);
    const overflow = await testPage.evaluate(
      () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
    );
    expect(overflow).toBe(false);
    await prCapture.screenshot("mobile-combined-diagnostic-logs", {
      caption: "The one diagnostic customizer remains touch-accessible on mobile.",
      fullPage: true,
    });
  });

  test("opens the customizer in an inset drawer with the standard source defaults", async ({
    testPage,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto("/settings/system/logs");

    const customize = testPage.getByTestId("customize-diagnostic-bundle");
    await expect(customize).toBeVisible();
    await customize.tap();

    const drawer = testPage.getByTestId("diagnostic-bundle-drawer");
    await expect(drawer).toBeVisible();
    await expect(drawer.getByRole("heading", { name: "Evidence sources" })).toBeVisible();
    await expect(drawer.getByRole("checkbox", { name: "Backend logs" })).toHaveAttribute(
      "data-state",
      "checked",
    );
    await expect(drawer.getByRole("checkbox", { name: "Frontend logs" })).toHaveAttribute(
      "data-state",
      "checked",
    );
    await expect(drawer.getByRole("checkbox", { name: "Runtime index" })).toHaveAttribute(
      "data-state",
      "unchecked",
    );
    const create = drawer.getByTestId("create-custom-diagnostic-bundle");
    expect((await create.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
      ),
    ).toBe(false);
  });
});
