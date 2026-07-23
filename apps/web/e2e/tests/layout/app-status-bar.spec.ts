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
    await expect
      .poll(() => bar.evaluate((element) => getComputedStyle(element).fontFamily))
      .toMatch(/^"?Geist"?/);

    const connectionDot = bar
      .locator('[data-status-item-id="builtin:connection"] [aria-hidden="true"]')
      .first();
    const dotBox = await connectionDot.boundingBox();
    if (!dotBox) throw new Error("connection dot has no bounding box");
    expect(Math.abs(dotBox.y + dotBox.height / 2 - (barBox.y + 12))).toBeLessThanOrEqual(0.5);
    await expect
      .poll(() =>
        bar.evaluate((element) => ({
          separatorHeight: getComputedStyle(element, "::before").height,
          contentHeight: getComputedStyle(element).height,
        })),
      )
      .toEqual({ separatorHeight: "1px", contentHeight: "24px" });
  });

  test("persists a modifier-mouse move across the spacer", async ({ testPage }) => {
    await testPage.goto("/");
    const bar = testPage.getByTestId("app-status-bar");
    const connection = bar.locator('[data-status-item-id="builtin:connection"]');
    const [sourceBox, barBox] = await Promise.all([connection.boundingBox(), bar.boundingBox()]);
    if (!sourceBox || !barBox) throw new Error("status bar drag geometry unavailable");
    const saved = testPage.waitForResponse(
      (response) =>
        response.request().method() === "PATCH" && response.url().endsWith("/api/v1/user/settings"),
    );

    await testPage.keyboard.down("Control");
    await testPage.mouse.move(
      sourceBox.x + sourceBox.width / 2,
      sourceBox.y + sourceBox.height / 2,
    );
    await testPage.mouse.down();
    await testPage.mouse.move(barBox.x + barBox.width - 8, barBox.y + barBox.height / 2, {
      steps: 8,
    });
    await testPage.mouse.up();
    await testPage.keyboard.up("Control");
    expect((await saved).ok()).toBe(true);

    await expect(connection).toHaveAttribute("data-status-side", "right");
    await testPage.reload();
    await expect(
      testPage.getByTestId("app-status-bar").locator('[data-status-item-id="builtin:connection"]'),
    ).toHaveAttribute("data-status-side", "right");
  });
});
