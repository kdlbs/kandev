import { test, expect } from "../../fixtures/test-base";
import {
  seedSizedImage,
  expectTouchTarget,
  expectImageDialogContained,
} from "./rich-output-image-helpers";
import {
  RICH_OUTPUT_FILE,
  RICH_OUTPUT_FILE_CONTENT,
  RICH_OUTPUT_IMAGE,
  seedRichOutputImage,
  seedRichOutputTask,
} from "./rich-output-helpers";

// @covers AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.1, AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.2,
// AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.3, AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.6
test("mobile image previews expand automatically with reachable hide and open actions", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  const session = await seedRichOutputTask({
    page: testPage,
    apiClient,
    seedData,
    title: "Mobile automatic image preview",
    payload: seedRichOutputImage(seedData.repositoryPath),
  });
  const report = session.activeChat().getByTestId("rich-output");
  const card = report.getByTestId("rich-output-file").filter({ hasText: RICH_OUTPUT_IMAGE });
  const toggle = card.getByTestId("rich-output-file-preview-toggle");
  const image = card.locator("img");
  await expect(toggle).toHaveAttribute("aria-expanded", "true");
  await expect(image).toBeVisible();
  await card.scrollIntoViewIfNeeded();
  await expect(toggle).toBeInViewport();
  await expect(card.getByTestId("rich-output-file-open")).toBeInViewport();
  await expect.poll(() => image.evaluate((img: HTMLImageElement) => img.naturalWidth)).toBe(640);
  await expect(
    report
      .getByTestId("rich-output-file")
      .filter({ hasText: RICH_OUTPUT_FILE })
      .getByTestId("rich-output-file-preview-toggle"),
  ).toHaveAttribute("aria-expanded", "false");
  const imageBox = await image.boundingBox();
  const cardBox = await card.boundingBox();
  expect(imageBox).not.toBeNull();
  expect(cardBox).not.toBeNull();
  expect(imageBox!.x).toBeGreaterThanOrEqual(cardBox!.x);
  expect(imageBox!.x + imageBox!.width).toBeLessThanOrEqual(cardBox!.x + cardBox!.width);
  expect(
    await testPage.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
  for (const button of await card.getByRole("button").all()) {
    expect((await button.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  }
  // @covers AC-AGENTS-IMAGE-PREVIEW-EXPANSION-001.1, AC-AGENTS-IMAGE-PREVIEW-EXPANSION-001.5
  const trigger = card.getByRole("button", { name: `Open ${RICH_OUTPUT_IMAGE}`, exact: true });
  await trigger.tap();
  const dialog = testPage.getByRole("dialog", { name: "Image preview" });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByRole("img")).toHaveAttribute("src", (await image.getAttribute("src"))!);
  await expect
    .poll(() => dialog.getByRole("img").evaluate((img: HTMLImageElement) => img.naturalWidth))
    .toBe(640);
  const close = await expectImageDialogContained(testPage);
  expect((await dialog.getByRole("img").boundingBox())!.width).toBeGreaterThan(imageBox!.width);
  await testPage.screenshot({ path: testInfo.outputPath("expanded-image-phone.png") });
  await close.tap();
  await expect(dialog).toBeHidden();
  await expect(trigger).toBeFocused();
  await expect(toggle).toHaveAttribute("aria-expanded", "true");
  await testPage.screenshot({ path: testInfo.outputPath("automatic-image-phone.png") });
  await toggle.tap();
  await expect(image).toHaveCount(0);
  await toggle.tap();
  await expect(image).toBeVisible();
  await card.getByTestId("rich-output-file-open").tap();
  const viewer = testPage.getByTestId("mobile-file-viewer-panel");
  await expect(viewer).toBeVisible();
  await expect(viewer.getByRole("img", { name: RICH_OUTPUT_IMAGE, exact: true })).toBeVisible();
});

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
  const checks = richOutput
    .getByTestId("rich-output-group")
    .filter({ has: testPage.getByRole("heading", { name: "Checks", exact: true }) });
  const interpretation = richOutput
    .getByTestId("rich-output-group")
    .filter({ has: testPage.getByRole("heading", { name: "Interpretation", exact: true }) });
  const [checksBox, interpretationBox] = await Promise.all([
    checks.boundingBox(),
    interpretation.boundingBox(),
  ]);
  expect(checksBox).not.toBeNull();
  expect(interpretationBox).not.toBeNull();
  expect(interpretationBox!.y).toBeGreaterThan(checksBox!.y + checksBox!.height);
  await checks.scrollIntoViewIfNeeded();
  await testPage.screenshot({ path: testInfo.outputPath("composed-phone.png") });
  const lineChart = richOutput.getByTestId("rich-output-chart-line");
  const barChart = richOutput.getByTestId("rich-output-chart-bar");
  await expect(lineChart).toBeVisible();
  await expect(barChart).toBeVisible();
  await lineChart.getByTestId("rich-output-chart-plot").scrollIntoViewIfNeeded();
  await expect(lineChart.locator(".recharts-xAxis text").first()).toBeVisible({ timeout: 15_000 });
  await expect(lineChart.locator(".recharts-yAxis text").first()).toBeVisible({ timeout: 15_000 });
  await barChart.getByTestId("rich-output-chart-plot").scrollIntoViewIfNeeded();
  await expect(barChart.locator(".recharts-xAxis text").first()).toBeVisible({ timeout: 15_000 });
  await expect(barChart.locator(".recharts-yAxis text").first()).toBeVisible({ timeout: 15_000 });
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
    const buttonBox = await button.boundingBox();
    expect(buttonBox).not.toBeNull();
    expect(buttonBox!.height).toBeGreaterThanOrEqual(44);
  }

  await richOutput.getByTestId("rich-output-file-preview-toggle").tap();
  await expect(richOutput).toContainText(RICH_OUTPUT_FILE_CONTENT.trim(), { timeout: 10_000 });

  await richOutput.getByTestId("rich-output-file-open").tap();
  const viewer = testPage.getByTestId("mobile-file-viewer-panel");
  await expect(viewer).toBeVisible({ timeout: 10_000 });
  await expect(viewer.getByTestId("mobile-file-viewer-content")).toContainText("checks=38");
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

// @covers AC-AGENTS-IMAGE-PREVIEW-EXPANSION-001.5
for (const size of [
  { width: 1, height: 1 },
  { width: 80, height: 1600 },
  { width: 1600, height: 128 },
]) {
  test(`mobile image inspection contains a ${size.width}x${size.height} image`, async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const payload = await seedSizedImage(testPage, seedData.repositoryPath, size);
    const session = await seedRichOutputTask({
      page: testPage,
      apiClient,
      seedData,
      title: "Image inspection geometry",
      payload,
    });
    const trigger = session
      .activeChat()
      .getByRole("button", { name: `Open ${RICH_OUTPUT_IMAGE}`, exact: true });
    await expect(trigger).toBeVisible();
    await expectTouchTarget(trigger);
    await trigger.tap();
    const close = await expectImageDialogContained(testPage);
    await close.tap();
    await expect(testPage.getByRole("dialog")).toBeHidden();
    await expect(trigger).toBeFocused();
  });
}
