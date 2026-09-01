import path from "node:path";
import type { Locator, Page } from "@playwright/test";
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

test.describe("Mobile topbar action strip", () => {
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

  test("scrolls plugin, metric, and native actions without moving fixed chrome", async ({
    testPage,
  }) => {
    test.setTimeout(120_000);

    await installFixture(testPage);
    await testPage.goto("/");
    await testPage.reload();

    const strip = testPage.getByTestId("mobile-topbar-action-strip");
    const viewport = testPage.getByTestId("mobile-topbar-action-strip-viewport");
    const leftFade = testPage.getByTestId("mobile-topbar-action-strip-left-fade");
    const rightFade = testPage.getByTestId("mobile-topbar-action-strip-right-fade");
    const brand = testPage.getByTestId("mobile-topbar-brand");
    const menu = testPage.getByTestId("mobile-topbar-menu");
    const pluginButton = testPage.locator("#hello-main-top-bar");
    const metrics = testPage.getByTestId("topbar-metrics");
    const cpuMetric = metrics.getByLabel(/^CPU /);
    const terminal = testPage.getByTestId("mobile-quick-terminal-button");
    const quickChat = testPage.getByTestId("mobile-quick-chat-button");
    const terminalHitTarget = testPage.getByTestId("mobile-quick-terminal-hit-target");
    const quickChatHitTarget = testPage.getByTestId("mobile-quick-chat-hit-target");
    const search = testPage.getByTestId("mobile-search-toggle");

    await expect(strip).toBeVisible();
    await expect(pluginButton).toBeVisible();
    await expect(metrics).toBeVisible();
    await expect(cpuMetric).toBeVisible();
    await expect(terminal).toBeVisible();
    await expect(quickChat).toBeVisible();
    await expect(search).toBeVisible();
    await expect(menu).toBeVisible();

    const nativeButtons = [terminal, quickChat, search, menu];
    const nativeBoxes = await Promise.all(
      nativeButtons.map((button, index) => requireBox(button, `native button ${index}`)),
    );
    for (const box of nativeBoxes) {
      expect(box.width).toBeCloseTo(32, 0);
      expect(box.height).toBeCloseTo(32, 0);
    }

    const pluginBox = await requireBox(pluginButton, "plugin action");
    expect(pluginBox.width).toBeCloseTo(32, 0);
    expect(pluginBox.height).toBeCloseTo(32, 0);

    const metricBox = await requireBox(metrics, "topbar metrics");
    expect(metricBox.height).toBeCloseTo(32, 0);
    const metricIconBox = await requireBox(cpuMetric.locator("svg").first(), "CPU metric icon");
    expect(metricIconBox.width).toBeCloseTo(16, 0);
    expect(metricIconBox.height).toBeCloseTo(16, 0);

    const pluginIconBox = await requireBox(pluginButton.locator("svg").first(), "plugin icon");
    expect(pluginIconBox.width).toBeCloseTo(16, 0);
    expect(pluginIconBox.height).toBeCloseTo(16, 0);

    const expectTouchTarget = async (target: Locator, label: string) => {
      const targetBox = await requireBox(target, `${label} hit target`);
      expect(targetBox.height).toBeCloseTo(44, 0);
      expect(targetBox.width).toBeCloseTo(32, 0);
      const hitTarget = await target.evaluate((element) => {
        const rect = element.getBoundingClientRect();
        const x = rect.left + rect.width / 2;
        const points = [rect.top + 1, rect.bottom - 1];
        return points.every((y) => document.elementFromPoint(x, y) === element);
      });
      expect(hitTarget, `${label} keeps a 44px hit area`).toBe(true);
    };

    const brandBefore = await requireBox(brand, "brand before scroll");
    const menuBefore = await requireBox(menu, "menu before scroll");
    await expect(strip).toHaveAttribute("data-can-scroll-left", "false");
    await expect(strip).toHaveAttribute("data-can-scroll-right", "true");
    await expect(leftFade).toHaveCount(0);
    await expect(rightFade).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "mobile topbar initial layout");

    await viewport.evaluate((element) => {
      const node = element as HTMLElement;
      node.scrollLeft = Math.floor((node.scrollWidth - node.clientWidth) / 2);
      node.dispatchEvent(new Event("scroll"));
    });
    await expect(strip).toHaveAttribute("data-can-scroll-left", "true");
    await expect(strip).toHaveAttribute("data-can-scroll-right", "true");
    await expect(leftFade).toBeVisible();
    await expect(rightFade).toBeVisible();

    await viewport.evaluate((element) => {
      const node = element as HTMLElement;
      node.scrollLeft = node.scrollWidth;
      node.dispatchEvent(new Event("scroll"));
    });
    await expect(strip).toHaveAttribute("data-can-scroll-left", "true");
    await expect(strip).toHaveAttribute("data-can-scroll-right", "false");
    await expect(leftFade).toBeVisible();
    await expect(rightFade).toHaveCount(0);
    await expect(search).toBeInViewport();
    // Touch targets are probed at the end position: with the title crumb on
    // the bar the native actions start beyond the strip's initial viewport,
    // and `document.elementFromPoint` can only hit what is scrolled into view.
    await expectTouchTarget(terminalHitTarget, "Quick Terminal");
    await expectTouchTarget(quickChatHitTarget, "Quick Chat");
    await assertNoDocumentHorizontalOverflow(testPage, "mobile topbar scrolled layout");

    const brandAfter = await requireBox(brand, "brand after scroll");
    const menuAfter = await requireBox(menu, "menu after scroll");
    expect(Math.abs(brandAfter.x - brandBefore.x)).toBeLessThanOrEqual(1);
    expect(Math.abs(menuAfter.x - menuBefore.x)).toBeLessThanOrEqual(1);

    await search.tap();
    await expect(testPage.getByTestId("mobile-search-bar")).toBeVisible();
    await expect(testPage.getByTestId("mobile-search-bar").getByRole("textbox")).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "mobile search layout");
  });
});
