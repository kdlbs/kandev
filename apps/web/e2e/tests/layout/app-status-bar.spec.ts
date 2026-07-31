import { expect, test } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";

const PIXEL_TOLERANCE = 1;

async function expectStatusBarAfterSidebar(page: Page) {
  const bar = page.getByTestId("app-status-bar");
  const sidebar = page.getByTestId("app-sidebar");
  const [barBox, sidebarBox, viewport] = await Promise.all([
    bar.boundingBox(),
    sidebar.boundingBox(),
    page.evaluate(() => ({ width: window.innerWidth, height: window.innerHeight })),
  ]);
  if (!barBox || !sidebarBox) throw new Error("app shell geometry unavailable");

  expect(Math.abs(sidebarBox.x)).toBeLessThanOrEqual(PIXEL_TOLERANCE);
  expect(Math.abs(barBox.x - (sidebarBox.x + sidebarBox.width))).toBeLessThanOrEqual(
    PIXEL_TOLERANCE,
  );
  expect(Math.abs(barBox.x + barBox.width - viewport.width)).toBeLessThanOrEqual(PIXEL_TOLERANCE);
  expect(Math.abs(sidebarBox.y + sidebarBox.height - viewport.height)).toBeLessThanOrEqual(
    PIXEL_TOLERANCE,
  );
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(viewport.width);

  return { barBox, viewport };
}

test.describe("App status bar", () => {
  test("starts after sidebar and tracks its layout width", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 1600, height: 900 });
    await testPage.goto("/");
    await setConnectionIssueSeverity(testPage, "unstable");

    const bar = testPage.getByTestId("app-status-bar");
    const sidebar = testPage.getByTestId("app-sidebar");
    await expect(bar).toBeVisible();
    await expect(sidebar).toBeVisible();

    const { barBox, viewport } = await expectStatusBarAfterSidebar(testPage);

    expect(barBox.height).toBe(24);
    expect(Math.abs(barBox.y + barBox.height - viewport.height)).toBeLessThanOrEqual(1);
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

    const resizeHandle = sidebar.getByRole("button", { name: "Resize sidebar" });
    const handleBox = await resizeHandle.boundingBox();
    if (!handleBox) throw new Error("sidebar resize handle has no bounding box");
    const handleCenter = handleBox.x + handleBox.width / 2;

    await testPage.mouse.move(handleCenter, handleBox.y + handleBox.height / 2);
    await testPage.mouse.down();
    await testPage.mouse.move(handleCenter + 80, handleBox.y + handleBox.height / 2);
    await testPage.mouse.up();
    await expect.poll(async () => (await sidebar.boundingBox())?.width ?? 0).toBeGreaterThan(350);
    await expectStatusBarAfterSidebar(testPage);

    await testPage.getByRole("button", { name: "Collapse sidebar" }).click();
    await expect(sidebar).toHaveAttribute("data-collapsed", "true");
    await expect.poll(async () => Math.round((await sidebar.boundingBox())?.width ?? 0)).toBe(56);
    await expectStatusBarAfterSidebar(testPage);
  });

  test("fills available width when responsive layout hides sidebar", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 700, height: 900 });
    await testPage.goto("/");

    const bar = testPage.getByTestId("app-status-bar");
    await expect(bar).toBeVisible();
    await expect(testPage.getByTestId("app-sidebar")).toBeHidden();

    const [barBox, viewport] = await Promise.all([
      bar.boundingBox(),
      testPage.evaluate(() => ({ width: window.innerWidth, height: window.innerHeight })),
    ]);
    if (!barBox) throw new Error("app status bar has no bounding box");

    expect(Math.abs(barBox.x)).toBeLessThanOrEqual(PIXEL_TOLERANCE);
    expect(Math.abs(barBox.width - viewport.width)).toBeLessThanOrEqual(PIXEL_TOLERANCE);
  });

  test("persists a modifier-mouse move across the spacer", async ({ testPage }) => {
    await testPage.goto("/");
    await setConnectionIssueSeverity(testPage, "unstable");
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

async function setConnectionIssueSeverity(page: Page, severity: "none" | "unstable" | "lost") {
  await page.evaluate((nextSeverity) => {
    const store = (
      window as Window & {
        __KANDEV_E2E_STORE__?: {
          getState: () => { setConnectionIssueSeverity: (severity: typeof nextSeverity) => void };
        };
      }
    ).__KANDEV_E2E_STORE__;
    if (!store) throw new Error("E2E store bridge missing");
    store.getState().setConnectionIssueSeverity(nextSeverity);
  }, severity);
}
