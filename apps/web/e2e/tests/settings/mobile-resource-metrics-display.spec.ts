import { expect, test } from "../../fixtures/test-base";
import {
  captureAppStatusBarSettings,
  restoreAppStatusBarSettings,
  setAppStatusBarEnabled,
  type AppStatusBarSettingsBaseline,
} from "../../helpers/app-status-bar-settings";
import {
  metricsUnavailableWasRendered,
  observeMetricsUnavailable,
} from "./metrics-loading-observer";
import type { SystemMetricId, SystemMetricsGlobalSettings } from "@/lib/types/system";

type SystemMetricsDisplay = {
  show_in_topbar: boolean;
  simplified?: boolean;
};

const ALL_HOST_METRICS = [
  "cpu_percent",
  "memory_percent",
  "disk_percent",
  "cpu_temp",
  "io_load",
] satisfies SystemMetricId[];

test.describe("Mobile resource metrics display", () => {
  let baseline: SystemMetricsDisplay;
  let statusBarBaseline: AppStatusBarSettingsBaseline;
  let globalMetricsBaseline: SystemMetricsGlobalSettings;

  test.beforeEach(async ({ apiClient }) => {
    const settings = await apiClient.getUserSettings();
    baseline = settings.settings.system_metrics_display as SystemMetricsDisplay;
    statusBarBaseline = await captureAppStatusBarSettings(apiClient);
    const globalMetricsResponse = await apiClient.rawRequest(
      "GET",
      "/api/v1/system/metrics/settings",
    );
    expect(globalMetricsResponse.ok).toBe(true);
    const globalMetrics = (await globalMetricsResponse.json()) as {
      settings: SystemMetricsGlobalSettings;
    };
    globalMetricsBaseline = globalMetrics.settings;
    await setAppStatusBarEnabled(apiClient, true);
  });

  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      system_metrics_display: baseline,
    });
    const globalMetricsRestore = await apiClient.rawRequest(
      "PATCH",
      "/api/v1/system/metrics/settings",
      globalMetricsBaseline,
    );
    expect(globalMetricsRestore.ok).toBe(true);
    await restoreAppStatusBarSettings(apiClient, statusBarBaseline);
  });

  test("stacks all enabled metrics inside the Status drawer", async ({ testPage, apiClient }) => {
    const globalMetricsUpdate = await apiClient.rawRequest(
      "PATCH",
      "/api/v1/system/metrics/settings",
      { ...globalMetricsBaseline, metrics: ALL_HOST_METRICS },
    );
    expect(globalMetricsUpdate.ok).toBe(true);
    const displayUpdate = await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      system_metrics_display: { show_in_topbar: true, simplified: false },
    });
    expect(displayUpdate.ok).toBe(true);

    await testPage.goto("/");
    await testPage.getByRole("button", { name: "Open menu" }).click();
    await testPage.getByTestId("mobile-home-status-button").click();

    const drawer = testPage.getByTestId("app-status-drawer");
    const metrics = drawer.getByTestId("app-status-metrics");
    await expect(metrics.getByLabel(/^CPU (?!temperature)/)).toBeVisible();
    const geometry = await metrics.evaluate((element) => {
      const indicators = Array.from(
        element.querySelectorAll<HTMLElement>('span[aria-label]:not([aria-label="Host metrics"])'),
      );
      const values = indicators[0]?.parentElement;
      const valuesRect = values?.getBoundingClientRect();
      const indicatorRects = indicators.map((indicator) => indicator.getBoundingClientRect());
      const rowTops: number[] = [];
      for (const rect of indicatorRects) {
        if (!rowTops.some((top) => Math.abs(top - rect.top) <= 1)) rowTops.push(rect.top);
      }
      const firstRowTop = rowTops[0];
      return {
        count: indicators.length,
        firstRowCount: indicatorRects.filter(
          (rect) => firstRowTop !== undefined && Math.abs(rect.top - firstRowTop) <= 1,
        ).length,
        rowCount: rowTops.length,
        valuesClientWidth: values?.clientWidth ?? 0,
        valuesScrollWidth: values?.scrollWidth ?? 0,
        contained:
          valuesRect !== undefined &&
          indicatorRects.every(
            (rect) => rect.left >= valuesRect.left - 1 && rect.right <= valuesRect.right + 1,
          ),
      };
    });

    expect(geometry.rowCount).toBe(3);
    expect(geometry.firstRowCount).toBe(2);
    expect(geometry.count).toBe(5);
    expect(geometry.contained).toBe(true);
    expect(geometry.valuesScrollWidth).toBeLessThanOrEqual(geometry.valuesClientWidth + 1);
    expect(await drawer.locator("[class*='overflow-y-auto']").count()).toBe(1);
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
      await testPage.evaluate(() => document.documentElement.clientWidth),
    );
  });

  test("renders simplified metrics in the Status drawer", async ({ testPage }) => {
    await testPage.goto("/settings");
    await testPage.getByTestId("settings-index").getByRole("link", { name: "Appearance" }).click();

    const showMetrics = testPage.getByRole("switch", { name: "Show host metrics in status bar" });
    const simplified = testPage.getByRole("switch", { name: "Simplified metrics" });
    if ((await showMetrics.getAttribute("aria-checked")) !== "true") await showMetrics.click();
    if ((await simplified.getAttribute("aria-checked")) !== "true") await simplified.click();

    await expect(simplified).toHaveAttribute("data-settings-dirty", "true");
    const floatingSave = testPage.getByTestId("settings-floating-save");
    await floatingSave.getByRole("button", { name: "Save changes" }).click();
    await expect(floatingSave).not.toBeVisible();
    await testPage.reload();
    await expect(simplified).toHaveAttribute("aria-checked", "true");

    await observeMetricsUnavailable(testPage);
    await testPage.goto("/");
    await testPage.getByRole("button", { name: "Open menu" }).click();
    await testPage.getByTestId("mobile-home-status-button").click();

    const drawer = testPage.getByTestId("app-status-drawer");
    const metrics = drawer.getByTestId("app-status-metrics");
    await expect(metrics.getByLabel(/^CPU /)).toBeVisible();
    expect(await metricsUnavailableWasRendered(testPage)).toBe(false);
    await expect(metrics.getByLabel("Host metrics")).toHaveCount(0);
    await expect(metrics.getByTestId("system-metric-meter")).toHaveCount(0);

    const metricsRow = drawer.locator('[data-status-item-id="builtin:metrics"]');
    const metricsRowHandle = await metricsRow.elementHandle();
    if (!metricsRowHandle) throw new Error("Status drawer metrics row unavailable");
    const geometry = await drawer.evaluate((element, row) => {
      const drawerRect = element.getBoundingClientRect();
      const rowRect = row.getBoundingClientRect();
      return {
        drawerBottom: drawerRect.bottom,
        drawerTop: drawerRect.top,
        rowHeight: rowRect.height,
      };
    }, metricsRowHandle);
    expect(geometry.rowHeight).toBeGreaterThanOrEqual(44);
    expect(geometry.drawerTop).toBeGreaterThanOrEqual(0);
    const dialog = testPage.getByRole("dialog", { name: "Status" });
    await expect(dialog).toHaveAttribute("data-state", "open");
    const viewportHeight = await testPage.evaluate(() => window.innerHeight);
    await expect
      .poll(() => dialog.evaluate((element) => element.getBoundingClientRect().bottom))
      .toBeLessThanOrEqual(viewportHeight);
    expect(await drawer.locator("[class*='overflow-y-auto']").count()).toBe(1);
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
      await testPage.evaluate(() => document.documentElement.clientWidth),
    );
  });
});
