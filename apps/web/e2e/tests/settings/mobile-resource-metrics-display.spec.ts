import { expect, test } from "../../fixtures/test-base";

type SystemMetricsDisplay = {
  show_in_topbar: boolean;
  simplified?: boolean;
};

test.describe("Mobile resource metrics display", () => {
  let baseline: SystemMetricsDisplay;

  test.beforeEach(async ({ apiClient }) => {
    const settings = await apiClient.getUserSettings();
    baseline = settings.settings.system_metrics_display as SystemMetricsDisplay;
  });

  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      system_metrics_display: baseline,
    });
  });

  test("renders simplified metrics in the Status drawer", async ({ testPage }) => {
    await testPage.goto("/settings/general/terminal");
    await testPage.getByTestId("settings-mobile-menu-button").click();
    await testPage
      .getByTestId("settings-mobile-menu")
      .getByRole("link", { name: "Appearance" })
      .click();

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

    await testPage.goto("/");
    await testPage.getByRole("button", { name: "Open menu" }).click();
    await testPage.getByTestId("mobile-home-status-button").click();

    const drawer = testPage.getByTestId("app-status-drawer");
    const metrics = drawer.getByTestId("app-status-metrics");
    await expect(metrics.getByLabel(/^CPU /)).toBeVisible();
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
