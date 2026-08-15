import { expect, test } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import {
  captureAppStatusBarSettings,
  restoreAppStatusBarSettings,
  setAppStatusBarEnabled,
  type AppStatusBarSettingsBaseline,
} from "../../helpers/app-status-bar-settings";

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
  let baseline: AppStatusBarSettingsBaseline | undefined;

  test.beforeEach(async ({ apiClient }) => {
    baseline = await captureAppStatusBarSettings(apiClient);
    await setAppStatusBarEnabled(apiClient, true);
  });

  test.afterEach(async ({ apiClient }) => {
    await restoreAppStatusBarSettings(apiClient, baseline);
  });

  test("persists the Appearance preference without a restart", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    await testPage.goto("/settings/preferences/appearance");
    const toggle = testPage.getByRole("switch", { name: "Show status bar" });
    const toggleRow = testPage.getByTestId("app-status-bar-toggle-row");
    const floatingSave = testPage.getByTestId("settings-floating-save");
    const bar = testPage.getByTestId("app-status-bar");

    await expect(toggle).toHaveAttribute("aria-checked", "true");
    expect((await toggleRow.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await expect(bar).toBeVisible();

    await toggle.click();
    await expect(toggle).toHaveAttribute("data-settings-dirty", "true");
    await floatingSave.getByRole("button", { name: "Save changes" }).click();
    await expect(bar).toHaveCount(0);
    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.app_status_bar_enabled)
      .toBe(false);
    await prCapture.screenshot("status-bar-appearance-desktop", {
      caption: "Desktop Appearance setting with the status bar disabled",
    });

    await testPage.reload();
    await expect(toggle).toHaveAttribute("aria-checked", "false");
    await expect(bar).toHaveCount(0);

    await toggle.click();
    await floatingSave.getByRole("button", { name: "Save changes" }).click();
    await expect(bar).toBeVisible();
    expect((await bar.boundingBox())?.height).toBe(24);
    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.app_status_bar_enabled)
      .toBe(true);
  });

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

  test("hands the status surface to the drawer once the sidebar is hidden", async ({
    testPage,
  }) => {
    // The sidebar is `hidden md:block` and `useResponsiveBreakpoint` switches to
    // mobile composition at the same 768px boundary, so no width renders the
    // inline bar without the sidebar beside it. Below the boundary the drawer
    // trigger *is* the status surface, and it has to stay reachable across the
    // whole band — 700px is the width where a 640px trigger class used to hide it.
    await testPage.setViewportSize({ width: 700, height: 900 });
    await testPage.goto("/stats");

    await expect(testPage.getByTestId("app-sidebar")).toBeHidden();
    await expect(testPage.getByTestId("app-status-bar")).toHaveCount(0);

    const trigger = testPage.getByTestId("app-status-drawer-trigger");
    await expect(trigger).toBeVisible();

    await trigger.click();
    await expect(testPage.getByTestId("app-status-drawer")).toBeVisible();
  });

  test("keeps the status drawer reachable on a coarse-pointer tablet outside tasks", async ({
    tabletTestPage,
  }) => {
    await tabletTestPage.goto("/stats");

    await expect(tabletTestPage.getByTestId("app-status-bar")).toHaveCount(0);
    const trigger = tabletTestPage.getByTestId("app-status-drawer-trigger");
    await expect(trigger).toBeVisible();
    await trigger.click();
    await expect(tabletTestPage.getByTestId("app-status-drawer")).toBeVisible();
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
