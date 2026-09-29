import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow, requireBox } from "../../helpers/layout-assertions";
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

type SystemMetricsDisplay = {
  show_in_topbar: boolean;
  simplified?: boolean;
};

test.describe("Resource metrics display", () => {
  let baseline: SystemMetricsDisplay;
  let statusBarBaseline: AppStatusBarSettingsBaseline;

  test.beforeEach(async ({ apiClient, testPage }) => {
    void testPage;
    const settings = await apiClient.getUserSettings();
    baseline = settings.settings.system_metrics_display as SystemMetricsDisplay;
    statusBarBaseline = await captureAppStatusBarSettings(apiClient);
    await setAppStatusBarEnabled(apiClient, true);
  });

  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      system_metrics_display: baseline,
    });
    await restoreAppStatusBarSettings(apiClient, statusBarBaseline);
  });

  test("keeps detailed desktop metrics in a compact inline status bar", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    const update = await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      system_metrics_display: { show_in_topbar: true, simplified: false },
    });
    expect(update.ok).toBe(true);
    await testPage.emulateMedia({ colorScheme: "dark" });
    await testPage.goto("/");
    const metrics = testPage.getByTestId("app-status-metrics");
    await expect(metrics.getByLabel(/^CPU /)).toBeVisible();
    await expect(metrics.getByRole("heading")).toHaveCount(0);
    const host = await requireBox(metrics.getByLabel("Host metrics"), "host badge");
    const cpu = await requireBox(metrics.getByLabel(/^CPU /), "CPU reading");
    expect(Math.abs(host.y + host.height / 2 - cpu.y - cpu.height / 2)).toBeLessThanOrEqual(1);
    expect((await requireBox(metrics, "desktop metrics")).height).toBeLessThanOrEqual(24);
    await assertNoDocumentHorizontalOverflow(testPage);
    if (prCapture.capturing) await expect(testPage.getByTestId("toast-message")).toHaveCount(0);
    await prCapture.screenshot("system-metrics-desktop", {
      caption: "Desktop retains the compact inline host metrics in the status bar",
    });
  });

  test("renders simplified metrics in the status bar", async ({ testPage }) => {
    await testPage.goto("/settings/preferences/appearance");
    const showMetrics = testPage.getByRole("switch", { name: "Show host metrics in status bar" });
    const simplified = testPage.getByRole("switch", { name: "Simplified metrics" });
    if ((await showMetrics.getAttribute("aria-checked")) !== "true") await showMetrics.click();
    if ((await simplified.getAttribute("aria-checked")) !== "true") await simplified.click();

    await expect(simplified).toHaveAttribute("data-settings-dirty", "true");
    const floatingSave = testPage.getByTestId("settings-floating-save");
    await floatingSave.getByRole("button", { name: "Save changes" }).click();
    await expect(floatingSave).not.toBeVisible();
    await observeMetricsUnavailable(testPage);
    await testPage.reload();
    await expect(simplified).toHaveAttribute("aria-checked", "true");
    await expect(testPage.getByTestId("app-status-metrics").getByLabel(/^CPU /)).toBeVisible();
    expect(await metricsUnavailableWasRendered(testPage)).toBe(false);

    await testPage.goto("/");

    const metrics = testPage.getByTestId("app-status-metrics");
    await expect(metrics.getByLabel(/^CPU /)).toBeVisible();
    await expect(metrics.getByLabel("Host metrics")).toHaveCount(0);
    await expect(metrics.getByTestId("system-metric-meter")).toHaveCount(0);
  });
});
