import { expect, test } from "../../fixtures/test-base";

type SystemMetricsDisplay = {
  show_in_topbar: boolean;
  simplified?: boolean;
};

test.describe("Resource metrics display", () => {
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

  test("renders simplified metrics in the status bar", async ({ testPage }) => {
    await testPage.goto("/settings/general/appearance");
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

    const metrics = testPage.getByTestId("app-status-metrics");
    await expect(metrics.getByLabel(/^CPU /)).toBeVisible();
    await expect(metrics.getByLabel("Host metrics")).toHaveCount(0);
    await expect(metrics.getByTestId("system-metric-meter")).toHaveCount(0);
  });
});
