/**
 * E2E: a plugin's own page is reachable from a phone.
 *
 * The desktop app sidebar (which renders `<PluginNavItems/>`) is
 * `hidden md:block`, so before `MobilePluginNavSection` existed a
 * `registerNavItem({ section: "main" })` entry had no phone entry point at
 * all — the route worked, but only if you typed the URL. Settings > Plugins
 * was always reachable through the settings menu sheet; this covers the
 * plugin's *own* nav item.
 */
import path from "node:path";
import type { Browser, BrowserContext, Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { PrAssetCapture } from "../../helpers/pr-asset-capture";

const PLUGIN_ID = "kandev-plugin-e2e";
const NAV_ITEM_ID = "e2e-hello";
const DESKTOP_VIEWPORT = { width: 1440, height: 900 };
const PACKAGE_PATH = path.resolve(
  __dirname,
  "../../../../../apps/backend/.build/kandev-plugin-e2e-1.0.0.tar.gz",
);

/** A desktop-sized client against the same backend, for the parity screenshot. */
async function openDesktopClient(
  browser: Browser,
  frontendUrl: string,
  backendPort: number,
): Promise<{ page: Page; context: BrowserContext }> {
  const context = await browser.newContext({
    baseURL: frontendUrl,
    viewport: DESKTOP_VIEWPORT,
  });
  const page = await context.newPage();
  await page.addInitScript(
    ({ port }: { port: number }) => {
      localStorage.setItem("kandev.onboarding.completed", "true");
      window.__KANDEV_API_PORT = String(port);
    },
    { port: backendPort },
  );
  return { page, context };
}

test.describe("Mobile plugin navigation", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
  });

  test("opens a plugin page from the phone menu sheet", async ({
    testPage,
    backend,
    browser,
  }, testInfo) => {
    test.setTimeout(120_000);
    const capture = new PrAssetCapture(testPage, testInfo.file);

    // Settings > Plugins is reachable on a phone through the settings menu
    // sheet — install the fixture through that same real flow.
    await testPage.goto("/settings/plugins");
    await testPage.getByTestId("install-plugin-trigger").click();
    await testPage.getByTestId("install-plugin-tab-upload").click();
    await testPage.getByTestId("install-plugin-file-input").setInputFiles(PACKAGE_PATH);
    await testPage.getByTestId("install-plugin-upload-submit").click();
    await expect(testPage.getByTestId(`plugin-row-${PLUGIN_ID}`)).toBeVisible({ timeout: 30_000 });

    await testPage.goto("/");
    await testPage.reload();

    // The desktop rail carries the same item but is hidden on a phone.
    await expect(testPage.getByTestId("app-sidebar")).toBeHidden();

    await testPage.getByRole("button", { name: "Open menu" }).click();
    const navItem = testPage.getByTestId(`mobile-plugin-nav-item-${NAV_ITEM_ID}`);
    await expect(navItem).toBeVisible();
    await expect(navItem).toHaveText(/Hello E2E/);
    expect((await navItem.boundingBox())?.height).toBeGreaterThanOrEqual(44);

    // The sheet slides up on open and the section sits below the fold, so
    // settle both before shooting or the asset captures an empty mid-animation
    // drawer instead of the change under test.
    await testPage.getByTestId("mobile-plugin-nav-section").scrollIntoViewIfNeeded();
    await expect(navItem).toBeInViewport();
    await capture.screenshot("mobile-plugin-nav-section", {
      caption: "Phone menu sheet: the plugin's page now has an entry point",
    });

    await navItem.click();
    await expect(testPage).toHaveURL(/\/plugins\/e2e-hello$/);
    await expect(testPage.locator("#hello-plugin-page")).toBeVisible();
    // Tapping the item also dismisses the sheet.
    await expect(testPage.getByTestId("mobile-plugin-nav-section")).toBeHidden();

    // Desktop parity reference: the same item in the always-visible sidebar.
    const desktop = await openDesktopClient(browser, backend.frontendUrl, backend.port);
    await desktop.page.goto("/");
    await desktop.page.waitForLoadState("networkidle");
    await expect(desktop.page.getByTestId(`plugin-nav-item-${NAV_ITEM_ID}`)).toBeVisible();
    await capture.screenshot("desktop-plugin-nav-sidebar", {
      page: desktop.page,
      caption: "Desktop, unchanged: the same nav item in the app sidebar",
    });
    await desktop.context.close();

    capture.flush();
  });
});
