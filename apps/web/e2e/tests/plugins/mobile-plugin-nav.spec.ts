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
import fs from "node:fs";
import path from "node:path";
import type { Browser, BrowserContext, Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { holdPluginInstallResponse } from "../../helpers/plugin-install";

const PLUGIN_ID = "kandev-plugin-e2e";
const NAV_ITEM_ID = "e2e-hello";
const DESKTOP_VIEWPORT = { width: 1440, height: 900 };
const PACKAGE_PATH = path.resolve(
  __dirname,
  "../../../../../apps/backend/.build/kandev-plugin-e2e-1.0.0.tar.gz",
);

function pluginExecutablePath(installPath: string): string {
  const serverDir = path.join(installPath, "server");
  const executable = fs
    .readdirSync(serverDir, { withFileTypes: true })
    .find((entry) => entry.isFile())?.name;
  if (!executable) throw new Error(`no plugin executable found under ${serverDir}`);
  return path.join(serverDir, executable);
}

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

  test("shows an install spinner while an upload install is pending on a phone", async ({
    testPage,
  }) => {
    test.setTimeout(120_000);

    await testPage.goto("/settings/plugins");
    await testPage.getByTestId("install-plugin-trigger").click();
    const heldInstall = await holdPluginInstallResponse(testPage);
    try {
      await testPage.getByTestId("install-plugin-tab-upload").tap();
      await testPage.getByTestId("install-plugin-file-input").setInputFiles(PACKAGE_PATH);
      await testPage.getByTestId("install-plugin-upload-submit").tap();
      await heldInstall.requestSeen;

      const installButton = testPage.getByTestId("install-plugin-upload-submit");
      await expect(installButton).toBeDisabled();
      await expect(installButton).toHaveAttribute("aria-busy", "true");
      await expect(installButton.locator(".animate-spin")).toBeVisible();
      await expect(installButton).toHaveText(/Installing/);
    } finally {
      heldInstall.release();
      if (heldInstall.requestStarted()) await heldInstall.responseSettled;
      await testPage.unroute("**/api/plugins/install");
    }

    const installButton = testPage.getByTestId("install-plugin-upload-submit");
    await expect(installButton).toBeEnabled();
    await expect(installButton).toHaveText("Install");
    await expect(testPage.getByTestId("install-plugin-error")).toContainText("install failed");
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

    // A plugin route with default chrome brings no nav of its own, and the
    // sidebar that normally owns Home is hidden below md — so the topbar must
    // carry a phone escape route or the user is stranded here.
    const home = testPage.getByTestId("topbar-phone-home");
    await expect(home).toBeVisible();
    await home.click();
    await expect(testPage).toHaveURL(/\/\?home=overview$/);

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

  test("retries an errored plugin from the phone settings row", async ({
    testPage,
    apiClient,
    backend,
  }) => {
    test.setTimeout(120_000);

    await testPage.goto("/settings/plugins");
    await testPage.getByTestId("install-plugin-trigger").click();
    await testPage.getByTestId("install-plugin-tab-upload").click();
    await testPage.getByTestId("install-plugin-file-input").setInputFiles(PACKAGE_PATH);
    await testPage.getByTestId("install-plugin-upload-submit").click();

    const pluginRow = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);
    await expect(pluginRow).toBeVisible({ timeout: 30_000 });
    const listResponse = await apiClient.rawRequest("GET", "/api/plugins");
    const listBody = (await listResponse.json()) as {
      plugins?: Array<{ id: string; install_path: string }>;
    };
    const installPath = listBody.plugins?.find((plugin) => plugin.id === PLUGIN_ID)?.install_path;
    if (!installPath) throw new Error(`plugin ${PLUGIN_ID} install path was not returned`);

    const executablePath = pluginExecutablePath(installPath);
    const unavailablePath = `${executablePath}.unavailable`;
    fs.renameSync(executablePath, unavailablePath);
    try {
      await backend.restart();
      await expect
        .poll(
          async () => {
            const response = await apiClient.rawRequest("GET", `/api/plugins/${PLUGIN_ID}`);
            if (!response.ok) return "missing";
            return ((await response.json()) as { status?: string }).status ?? "missing";
          },
          { timeout: 20_000, intervals: [250, 500, 1000] },
        )
        .toBe("error");

      await testPage.goto("/settings/plugins");
      await expect(pluginRow.getByText("Error", { exact: true })).toBeVisible({ timeout: 15_000 });
      await expect(pluginRow.getByTestId(`plugin-error-${PLUGIN_ID}`)).toBeVisible();
      await expect(pluginRow.getByRole("button", { name: "Enable" })).toBeVisible();
      expect(
        (await pluginRow.getByRole("button", { name: "Enable" }).boundingBox())?.height,
      ).toBeGreaterThanOrEqual(44);

      fs.renameSync(unavailablePath, executablePath);
      await pluginRow.getByRole("button", { name: "Enable" }).click();
      await expect(pluginRow.getByText("Active", { exact: true })).toBeVisible({ timeout: 15_000 });
      await expect(pluginRow.getByTestId(`plugin-error-${PLUGIN_ID}`)).toHaveCount(0);
    } finally {
      if (fs.existsSync(unavailablePath) && !fs.existsSync(executablePath)) {
        fs.renameSync(unavailablePath, executablePath);
      }
    }
  });

  test("wraps a long plugin diagnostic without phone horizontal overflow", async ({
    testPage,
    apiClient,
    backend,
  }) => {
    test.setTimeout(120_000);

    await testPage.goto("/settings/plugins");
    await testPage.getByTestId("install-plugin-trigger").click();
    await testPage.getByTestId("install-plugin-tab-upload").click();
    await testPage.getByTestId("install-plugin-file-input").setInputFiles(PACKAGE_PATH);
    await testPage.getByTestId("install-plugin-upload-submit").click();

    const pluginRow = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);
    await expect(pluginRow).toBeVisible({ timeout: 30_000 });
    const listResponse = await apiClient.rawRequest("GET", "/api/plugins");
    const listBody = (await listResponse.json()) as {
      plugins?: Array<{ id: string; install_path: string }>;
    };
    const installPath = listBody.plugins?.find((plugin) => plugin.id === PLUGIN_ID)?.install_path;
    if (!installPath) throw new Error(`plugin ${PLUGIN_ID} install path was not returned`);

    const executablePath = pluginExecutablePath(installPath);
    const unavailablePath = `${executablePath}.unavailable`;
    fs.renameSync(executablePath, unavailablePath);
    const longDiagnostic = "X".repeat(2048);

    try {
      await backend.restart();
      await expect
        .poll(
          async () => {
            const response = await apiClient.rawRequest("GET", `/api/plugins/${PLUGIN_ID}`);
            if (!response.ok) return "missing";
            return ((await response.json()) as { status?: string }).status ?? "missing";
          },
          { timeout: 20_000, intervals: [250, 500, 1000] },
        )
        .toBe("error");

      await testPage.route("**/api/plugins", async (route) => {
        const response = await route.fetch();
        const body = (await response.json()) as {
          plugins?: Array<Record<string, unknown> & { id?: string }>;
        };
        body.plugins = body.plugins?.map((plugin) =>
          plugin.id === PLUGIN_ID
            ? {
                ...plugin,
                last_error: longDiagnostic,
                last_error_at: "2026-08-02T12:34:56Z",
              }
            : plugin,
        );
        await route.fulfill({ response, json: body });
      });

      await testPage.reload();
      const diagnostic = pluginRow.getByTestId(`plugin-error-${PLUGIN_ID}`);
      await expect(diagnostic).toBeVisible({ timeout: 15_000 });
      await expect(diagnostic.locator("span")).toHaveText(longDiagnostic);

      const hasHorizontalOverflow = await testPage.evaluate(
        () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
      );
      expect(hasHorizontalOverflow).toBe(false);
    } finally {
      await testPage.unroute("**/api/plugins").catch(() => undefined);
      if (fs.existsSync(unavailablePath) && !fs.existsSync(executablePath)) {
        fs.renameSync(unavailablePath, executablePath);
      }
    }
  });
});
