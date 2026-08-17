import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import {
  RICH_OUTPUT_FILE,
  RICH_OUTPUT_FILE_CONTENT,
  seedRichOutputTask,
} from "./rich-output-helpers";

test("renders and persists native rich output with an explicit file preview", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const session = await seedRichOutputTask({
    page: testPage,
    apiClient,
    seedData,
    title: "Desktop rich output",
  });
  const richOutput = session.activeChat().getByTestId("rich-output");

  await expect(richOutput).toBeVisible({ timeout: 30_000 });
  await expect(richOutput.getByRole("heading", { name: "Release health" })).toBeVisible();
  await expect(richOutput.getByTestId("rich-output-metrics")).toContainText("38");
  const lineChart = richOutput.getByTestId("rich-output-chart-line");
  const barChart = richOutput.getByTestId("rich-output-chart-bar");
  await expect(lineChart).toBeVisible();
  await expect(barChart).toBeVisible();
  await lineChart.scrollIntoViewIfNeeded();
  await expect(lineChart.locator(".recharts-xAxis text").first()).toBeVisible();
  await expect(lineChart.locator(".recharts-line-curve")).toHaveAttribute("stroke-dasharray", /\d/);
  await expect(lineChart.locator(".recharts-yAxis text").first()).toBeVisible();
  await expect(lineChart.locator(".recharts-xAxis")).toContainText("Aug 12");
  await barChart.scrollIntoViewIfNeeded();
  await expect(barChart.locator(".recharts-xAxis text").first()).toBeVisible();
  await expect(barChart.locator(".recharts-yAxis text").first()).toBeVisible();
  await expect(barChart.locator(".recharts-xAxis")).toContainText("/api");

  const singleSeriesLegend = lineChart.getByTestId("rich-output-chart-legend-series_0");
  await expect(singleSeriesLegend).toContainText("p95");
  await expect(singleSeriesLegend).not.toHaveAttribute("role", "button");

  const errorsLegend = barChart.getByRole("button", { name: "Errors" });
  const visibleBars = barChart.locator(".recharts-bar-rectangle");
  await expect(barChart.getByRole("button", { name: "Requests" })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await expect(errorsLegend).toHaveAttribute("aria-pressed", "true");
  await expect(visibleBars).toHaveCount(6);

  await barChart.locator(".recharts-bar-rectangle").first().hover();
  const tooltip = barChart.locator(".recharts-tooltip-wrapper");
  await expect(tooltip).toBeVisible();
  await expect(tooltip).toContainText("/api");
  await expect(tooltip).toContainText("2,400");

  await errorsLegend.click();
  await expect(errorsLegend).toHaveAttribute("aria-pressed", "false");
  await expect(visibleBars).toHaveCount(3);
  await errorsLegend.click();
  await expect(errorsLegend).toHaveAttribute("aria-pressed", "true");
  await expect(visibleBars).toHaveCount(6);
  await expect(richOutput.getByText("Raw verification report")).toBeVisible();
  await expect(richOutput).not.toContainText("RICH_OUTPUT_FILE_PREVIEW");

  fs.rmSync(path.join(seedData.repositoryPath, "rich-output-chart-data.csv"), { force: true });
  await testPage.reload();
  await session.waitForLoad();
  await expect(session.activeChat().getByTestId("rich-output")).toContainText("Release health", {
    timeout: 30_000,
  });

  const persisted = session.activeChat().getByTestId("rich-output");
  await expect(persisted.getByTestId("rich-output-chart-line")).toBeVisible();
  await expect(persisted.getByTestId("rich-output-chart-bar")).toBeVisible();
  await persisted.getByTestId("rich-output-chart-line").scrollIntoViewIfNeeded();
  await expect(
    persisted.getByTestId("rich-output-chart-line").locator(".recharts-xAxis text").first(),
  ).toBeVisible();
  await expect(
    persisted
      .getByTestId("rich-output-chart-line")
      .getByTestId("rich-output-chart-legend-series_0"),
  ).toContainText("p95");
  await persisted.getByTestId("rich-output-chart-bar").scrollIntoViewIfNeeded();
  await expect(persisted.getByRole("button", { name: "Errors" })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await persisted.getByTestId("rich-output-file-preview-toggle").click();
  await expect(persisted).toContainText(RICH_OUTPUT_FILE_CONTENT.trim(), { timeout: 10_000 });

  await persisted.getByTestId("rich-output-file-open").click();
  await expect(testPage.locator(`.dv-default-tab:has-text('${RICH_OUTPUT_FILE}')`)).toBeVisible({
    timeout: 10_000,
  });
});

test("renders complete chart geometry when device animation is disabled", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await testPage.addInitScript(() => {
    window.localStorage.setItem("kandev.settings.richOutputAnimations", "false");
  });
  const session = await seedRichOutputTask({
    page: testPage,
    apiClient,
    seedData,
    title: "Static desktop rich output",
  });
  const richOutput = session.activeChat().getByTestId("rich-output");
  const lineChart = richOutput.getByTestId("rich-output-chart-line");
  const barChart = richOutput.getByTestId("rich-output-chart-bar");

  await lineChart.scrollIntoViewIfNeeded();
  const line = lineChart.locator(".recharts-line-curve");
  await expect(line).toBeVisible({ timeout: 30_000 });
  await expect(line).not.toHaveAttribute("stroke-dasharray", /\d/);

  await barChart.scrollIntoViewIfNeeded();
  await expect(barChart.locator(".recharts-bar-rectangle")).toHaveCount(6);
});
