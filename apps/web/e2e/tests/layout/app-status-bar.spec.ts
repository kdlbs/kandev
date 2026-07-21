import { expect, test } from "../../fixtures/test-base";

test.describe("App status bar", () => {
  test("uses one 24px in-flow footer across sidebar and route content", async ({ testPage }) => {
    await testPage.goto("/");

    const bar = testPage.getByTestId("app-status-bar");
    await expect(bar).toBeVisible();
    await expect(testPage.getByTestId("app-sidebar")).toBeVisible();

    const [barBox, viewport] = await Promise.all([
      bar.boundingBox(),
      testPage.evaluate(() => ({ width: window.innerWidth, height: window.innerHeight })),
    ]);
    if (!barBox) throw new Error("app status bar has no bounding box");

    expect(barBox.height).toBe(24);
    expect(Math.abs(barBox.y + barBox.height - viewport.height)).toBeLessThanOrEqual(1);
    expect(Math.abs(barBox.x)).toBeLessThanOrEqual(1);
    expect(Math.abs(barBox.width - viewport.width)).toBeLessThanOrEqual(1);
  });
});
