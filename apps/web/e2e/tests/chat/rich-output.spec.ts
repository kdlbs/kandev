import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import type { Locator } from "@playwright/test";
import {
  RICH_OUTPUT_FILE,
  RICH_OUTPUT_FILE_CONTENT,
  RICH_OUTPUT_IMAGE,
  seedRichOutputImage,
  seedRichOutputTask,
} from "./rich-output-helpers";

// @covers AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.1, AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.2,
// AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.3, AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.4
test("image file previews expand automatically while text remains lazy", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const session = await seedRichOutputTask({
    page: testPage,
    apiClient,
    seedData,
    title: "Automatic image preview",
    payload: seedRichOutputImage(seedData.repositoryPath),
  });
  const report = session.activeChat().getByTestId("rich-output");
  const imageCard = report.getByTestId("rich-output-file").filter({ hasText: RICH_OUTPUT_IMAGE });
  const textCard = report.getByTestId("rich-output-file").filter({ hasText: RICH_OUTPUT_FILE });
  const toggle = imageCard.getByTestId("rich-output-file-preview-toggle");
  const image = imageCard.getByRole("img", { name: RICH_OUTPUT_IMAGE, exact: true });
  await expect(toggle).toHaveAttribute("aria-expanded", "true");
  await expect(image).toBeVisible();
  await expect.poll(() => image.evaluate((img: HTMLImageElement) => img.naturalWidth)).toBe(640);
  await expect(textCard.getByTestId("rich-output-file-preview-toggle")).toHaveAttribute(
    "aria-expanded",
    "false",
  );
  await expect(textCard).not.toContainText(RICH_OUTPUT_FILE_CONTENT.trim());
  await toggle.click();
  await expect(image).toHaveCount(0);
  await testPage.setViewportSize({ width: 390, height: 844 });
  await expect(toggle).toHaveAttribute("aria-expanded", "false");
  await testPage.setViewportSize({ width: 1280, height: 900 });
  await expect(toggle).toHaveAttribute("aria-expanded", "false");
  await toggle.press("Space");
  await expect(image).toBeVisible();
  await imageCard.getByTestId("rich-output-file-open").click();
  await expect(testPage.locator(`.dv-default-tab:has-text('${RICH_OUTPUT_IMAGE}')`)).toBeVisible();
});

async function revealChartPlot(chart: Locator): Promise<void> {
  await chart.getByTestId("rich-output-chart-plot").scrollIntoViewIfNeeded();
}

test("renders and persists native rich output with an explicit file preview", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
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
  const checks = richOutput
    .getByTestId("rich-output-group")
    .filter({ has: testPage.getByRole("heading", { name: "Checks", exact: true }) });
  const interpretation = richOutput
    .getByTestId("rich-output-group")
    .filter({ has: testPage.getByRole("heading", { name: "Interpretation", exact: true }) });
  await expect(checks).toBeVisible();
  await expect(interpretation).toContainText("Charts retain accepted CSV data");
  const [checksBox, interpretationBox] = await Promise.all([
    checks.boundingBox(),
    interpretation.boundingBox(),
  ]);
  expect(checksBox).not.toBeNull();
  expect(interpretationBox).not.toBeNull();
  expect(interpretationBox!.x).toBeGreaterThan(checksBox!.x);
  await checks.scrollIntoViewIfNeeded();
  await testPage.screenshot({ path: testInfo.outputPath("composed-desktop.png") });
  const lineChart = richOutput.getByTestId("rich-output-chart-line");
  const barChart = richOutput.getByTestId("rich-output-chart-bar");
  const barPlot = barChart.getByTestId("rich-output-chart-plot");
  await expect(lineChart).toBeVisible();
  await expect(barChart).toBeVisible();
  await revealChartPlot(lineChart);
  await expect(lineChart.locator(".recharts-xAxis text").first()).toBeVisible();
  await expect(lineChart.locator(".recharts-line-curve")).toHaveAttribute("d", /^M[\d.,-]+[CL]/);
  await expect(lineChart.locator(".recharts-yAxis text").first()).toBeVisible();
  await expect(lineChart.locator(".recharts-xAxis")).toContainText("Aug 12");
  // Scroll the observed plot once. Repeating scroll actions while the lazy
  // chart mounts can keep the virtualized transcript in motion indefinitely.
  await barPlot.scrollIntoViewIfNeeded();
  await expect(barPlot).toBeInViewport();
  await expect(barPlot.locator("svg")).toBeVisible({ timeout: 30_000 });
  await expect(barChart.locator(".recharts-xAxis text").first()).toBeVisible({ timeout: 30_000 });
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
  await revealChartPlot(persisted.getByTestId("rich-output-chart-line"));
  await expect(
    persisted.getByTestId("rich-output-chart-line").locator(".recharts-xAxis text").first(),
  ).toBeVisible();
  await expect(
    persisted
      .getByTestId("rich-output-chart-line")
      .getByTestId("rich-output-chart-legend-series_0"),
  ).toContainText("p95");
  await revealChartPlot(persisted.getByTestId("rich-output-chart-bar"));
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
    // This case checks static Recharts geometry. The lazy plot mount has its
    // own coverage above; make visibility callbacks immediate here so the
    // animation assertion cannot race with an inner transcript scroll.
    class ImmediateIntersectionObserver {
      private readonly callback: IntersectionObserverCallback;

      constructor(callback: IntersectionObserverCallback) {
        this.callback = callback;
      }

      observe(target: Element) {
        this.callback(
          [
            {
              isIntersecting: true,
              intersectionRatio: 1,
              target,
              boundingClientRect: target.getBoundingClientRect(),
              intersectionRect: target.getBoundingClientRect(),
              rootBounds: null,
              time: performance.now(),
            } as IntersectionObserverEntry,
          ],
          this as unknown as IntersectionObserver,
        );
      }

      unobserve() {}
      disconnect() {}
      takeRecords(): IntersectionObserverEntry[] {
        return [];
      }
    }
    Object.defineProperty(window, "IntersectionObserver", {
      configurable: true,
      value: ImmediateIntersectionObserver,
    });
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
  const barPlot = barChart.getByTestId("rich-output-chart-plot");

  await expect(richOutput).toBeVisible({ timeout: 30_000 });
  await expect(barChart).toBeVisible({ timeout: 30_000 });
  await lineChart.scrollIntoViewIfNeeded();
  const line = lineChart.locator(".recharts-line-curve");
  await expect(line).toBeVisible({ timeout: 30_000 });
  await expect(line).toHaveAttribute("d", /^M[\d.,-]+[CL]/);
  await expect(line).not.toHaveAttribute("stroke-dasharray", /\d/);

  await barPlot.evaluate((element) =>
    element.scrollIntoView({ block: "center", inline: "nearest", behavior: "instant" }),
  );
  await expect(barPlot.locator("svg")).toBeVisible({ timeout: 30_000 });
  await expect(barPlot.locator(".recharts-bar-rectangle")).toHaveCount(6, {
    timeout: 30_000,
  });
});

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-002.4, AC-AGENTS-AGENT-RICH-OUTPUT-002.6
test("replays a historical CSV report without its source file", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const session = await seedRichOutputTask({
    page: testPage,
    apiClient,
    seedData,
    title: "Historical rich output",
    historical: true,
  });
  const report = session.activeChat().getByTestId("rich-output");
  await expect(report).toHaveCount(1);
  await expect(report).toContainText("Historical health", { timeout: 30_000 });
  await expect(report.getByTestId("rich-output-chart-bar")).toContainText("Stored CSV sample");
  await expect(report.getByTestId("rich-output-metrics")).toContainText("38");
  await report.getByTestId("rich-output-file-preview-toggle").click();
  await expect(report).toContainText(RICH_OUTPUT_FILE_CONTENT.trim());
  await testPage.reload();
  await session.waitForLoad();
  await expect(session.activeChat().getByTestId("rich-output")).toContainText("Historical health");
});

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-001.18, AC-AGENTS-AGENT-RICH-OUTPUT-002.8
test("narrow mouse-input layouts use phone composition and touch-sized controls", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  await testPage.setViewportSize({ width: 390, height: 844 });
  const session = await seedRichOutputTask({
    page: testPage,
    apiClient,
    seedData,
    title: "Narrow rich output",
  });
  expect(await testPage.evaluate(() => matchMedia("(pointer: fine)").matches)).toBe(true);
  const report = session.activeChat().getByTestId("rich-output");
  await expect(report).toHaveCount(1);
  const groups = report.getByTestId("rich-output-group");
  await expect(groups).toHaveCount(2);
  const [first, second] = await Promise.all([
    groups.nth(0).boundingBox(),
    groups.nth(1).boundingBox(),
  ]);
  expect(first).not.toBeNull();
  expect(second).not.toBeNull();
  expect(second!.y).toBeGreaterThan(first!.y + first!.height);
  const toggle = report.getByTestId("rich-output-file-preview-toggle");
  await toggle.scrollIntoViewIfNeeded();
  const box = await toggle.boundingBox();
  expect(box).not.toBeNull();
  expect(box!.height).toBeGreaterThanOrEqual(44);
  await toggle.click();
  await expect(report).toContainText(RICH_OUTPUT_FILE_CONTENT.trim());
  await testPage.screenshot({ path: testInfo.outputPath("narrow-fine-pointer.png") });
});
