import { test, expect } from "../../fixtures/test-base";
import { RICH_OUTPUT_FILE_CONTENT, seedRichOutputTask } from "./rich-output-helpers";

test("mobile rich output stays contained and opens the native file viewer", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  test.skip(testInfo.project.name !== "mobile-chrome", "mobile-only rich output layout");
  const session = await seedRichOutputTask({
    page: testPage,
    apiClient,
    seedData,
    title: "Mobile rich output",
  });
  const richOutput = session.activeChat().getByTestId("rich-output");
  const charts = richOutput.locator('figure[data-testid^="rich-output-chart-"]');

  await expect(richOutput).toBeVisible({ timeout: 30_000 });
  await expect(charts).toHaveCount(2);
  const lineChart = richOutput.getByTestId("rich-output-chart-line");
  const barChart = richOutput.getByTestId("rich-output-chart-bar");
  await expect(lineChart).toBeVisible();
  await expect(barChart).toBeVisible();
  await lineChart.scrollIntoViewIfNeeded();
  await expect(lineChart.locator(".recharts-xAxis text").first()).toBeVisible();
  await expect(lineChart.locator(".recharts-yAxis text").first()).toBeVisible();
  await barChart.scrollIntoViewIfNeeded();
  await expect(barChart.locator(".recharts-xAxis text").first()).toBeVisible();
  await expect(barChart.locator(".recharts-yAxis text").first()).toBeVisible();
  await expect(lineChart.getByTestId("rich-output-chart-legend-series_0")).toContainText("p95");

  const errorsLegend = barChart.getByRole("button", { name: "Errors" });
  const visibleBars = barChart.locator(".recharts-bar-rectangle");
  await expect(errorsLegend).toHaveAttribute("aria-pressed", "true");
  await expect(visibleBars).toHaveCount(6);
  await errorsLegend.tap();
  await expect(errorsLegend).toHaveAttribute("aria-pressed", "false");
  await expect(visibleBars).toHaveCount(3);
  await errorsLegend.tap();
  await expect(errorsLegend).toHaveAttribute("aria-pressed", "true");
  await expect(visibleBars).toHaveCount(6);
  await expect
    .poll(() =>
      testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    )
    .toBe(true);

  const cardBox = await richOutput.boundingBox();
  expect(cardBox).not.toBeNull();
  for (const chart of await charts.all()) {
    const chartBox = await chart.boundingBox();
    expect(chartBox).not.toBeNull();
    expect(chartBox!.x).toBeGreaterThanOrEqual(cardBox!.x);
    expect(chartBox!.x + chartBox!.width).toBeLessThanOrEqual(cardBox!.x + cardBox!.width + 1);
  }

  for (const button of await richOutput.getByRole("button").all()) {
    expect(await button.evaluate((element) => element.offsetHeight)).toBeGreaterThanOrEqual(44);
  }

  await richOutput.getByTestId("rich-output-file-preview-toggle").tap();
  await expect(richOutput).toContainText(RICH_OUTPUT_FILE_CONTENT.trim(), { timeout: 10_000 });
  await richOutput.getByTestId("rich-output-file-open").tap();
  const viewer = testPage.getByTestId("mobile-file-viewer-panel");
  await expect(viewer).toBeVisible({ timeout: 10_000 });
  await expect(viewer.getByTestId("mobile-file-viewer-content")).toContainText("checks=38");
});
