import path from "node:path";
import type { Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import {
  captureAppStatusBarSettings,
  restoreAppStatusBarSettings,
  type AppStatusBarSettingsBaseline,
} from "../../helpers/app-status-bar-settings";
import { assertNoDocumentHorizontalOverflow, requireBox } from "../../helpers/layout-assertions";

const PLUGIN_ID = "kandev-plugin-e2e";
const PACKAGE_PATH = path.resolve(
  __dirname,
  "../../../../../apps/backend/.build/kandev-plugin-e2e-1.0.0.tar.gz",
);

type SystemMetricsDisplayBaseline = {
  show_in_topbar: boolean;
  simplified?: boolean;
};

async function installFixture(page: Page) {
  await page.goto("/settings/plugins");
  await page.getByTestId("install-plugin-trigger").tap();
  await page.getByTestId("install-plugin-tab-upload").tap();
  await page.getByTestId("install-plugin-file-input").setInputFiles(PACKAGE_PATH);
  await page.getByTestId("install-plugin-upload-submit").tap();
  await expect(page.getByTestId(`plugin-row-${PLUGIN_ID}`)).toBeVisible({ timeout: 30_000 });
}

test.describe("Mobile listing menu actions", () => {
  let metricsBaseline: SystemMetricsDisplayBaseline;
  let statusBarBaseline: AppStatusBarSettingsBaseline;

  test.beforeEach(async ({ apiClient, testPage }) => {
    void testPage;
    const settings = await apiClient.getUserSettings();
    metricsBaseline = (settings.settings.system_metrics_display as
      | SystemMetricsDisplayBaseline
      | undefined) ?? { show_in_topbar: false };
    statusBarBaseline = await captureAppStatusBarSettings(apiClient);

    const response = await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      app_status_bar_enabled: false,
      system_metrics_display: { show_in_topbar: true, simplified: false },
    });
    expect(response.ok).toBe(true);
  });

  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
    await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      system_metrics_display: metricsBaseline,
    });
    await restoreAppStatusBarSettings(apiClient, statusBarBaseline);
  });

  test("keeps plugins, opted-in metrics, and native tools accessible in every mode", async ({
    testPage,
  }) => {
    test.setTimeout(120_000);
    await installFixture(testPage);
    for (const [route, pluginPage] of [
      ["/?home=overview", "kanban"],
      ["/tasks", "tasks"],
      ["/threads", "kanban"],
    ]) {
      await testPage.goto(route);
      await expect(testPage.getByTestId("mobile-topbar-action-strip")).toHaveCount(0);
      await expect(testPage.getByTestId("app-status-metrics")).toHaveCount(0);
      await testPage.getByTestId("mobile-topbar-menu").tap();
      const menu = testPage.getByRole("dialog", { name: "Menu", exact: true });
      const plugin = menu.locator("#hello-main-top-bar");
      const metrics = menu.getByTestId("app-status-metrics");
      await expect(plugin).toHaveAccessibleName(`Hello ${pluginPage}`);
      await expect(metrics).toBeVisible();
      await expect(metrics.getByLabel(/^CPU /)).toBeVisible();
      for (const target of [
        plugin,
        menu.getByTestId("mobile-quick-chat-button"),
        menu.getByTestId("mobile-quick-terminal-button"),
      ]) {
        const box = await requireBox(target, "menu action");
        expect(box.height).toBeGreaterThanOrEqual(44);
        expect(box.width).toBeGreaterThanOrEqual(44);
        expect(box.x + box.width).toBeLessThanOrEqual(testPage.viewportSize()!.width);
      }
      const icon = await requireBox(plugin.locator("svg").first(), "plugin icon");
      expect(icon.width).toBeCloseTo(16, 0);
      expect(icon.height).toBeCloseTo(16, 0);
      await assertNoDocumentHorizontalOverflow(testPage, `menu tools on ${route}`);
      if (route !== "/threads") {
        await expect(menu.getByRole("textbox")).toHaveCount(0);
        await menu.getByTestId("mobile-search-toggle").tap();
        const search = testPage.getByTestId("mobile-search-bar");
        await expect(menu).toHaveCount(0);
        await expect(search.getByRole("textbox")).toBeFocused();
        await assertNoDocumentHorizontalOverflow(testPage, "phone search");
      } else {
        await testPage.keyboard.press("Escape");
      }
      await expect(testPage.getByTestId("app-status-metrics")).toHaveCount(0);
    }
  });
});
