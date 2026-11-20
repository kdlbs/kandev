import { expect, test } from "../../fixtures/test-base";
import { RICH_OUTPUT_FILE_CONTENT, seedRichOutputTask } from "./rich-output-helpers";

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-001.21, AC-AGENTS-AGENT-RICH-OUTPUT-001.23
test("discloses a retained rich-output failure beside its successful retry", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}, testInfo) => {
  const session = await seedRichOutputTask({
    page: testPage,
    apiClient,
    seedData,
    title: "Rich output error disclosure",
    failureRetry: true,
  });
  const chat = session.activeChat();
  const report = chat.getByTestId("rich-output");
  const trigger = chat.getByTestId("rich-output-error-trigger");

  await expect(report.getByRole("heading", { name: "Release health" })).toBeVisible();
  await expect(report).toContainText("Corrected verification");
  await expect(trigger).toHaveCount(1);
  await expect(trigger).toHaveCSS("cursor", "pointer");
  const fileToggle = report.getByTestId("rich-output-file-preview-toggle");
  await fileToggle.click();
  await expect(report).toContainText(RICH_OUTPUT_FILE_CONTENT.trim());
  const chartFilter = report.getByTestId("rich-output-chart-legend-series_0");
  await chartFilter.click();
  await expect(chartFilter).toHaveAttribute("aria-pressed", "false");

  const tooltip = testPage.getByTestId("rich-output-error-tooltip");
  await trigger.hover();
  await expect(tooltip).toBeVisible();
  await testPage.mouse.move(0, 0);
  await expect(tooltip).toBeHidden();
  await trigger.focus();
  await expect(tooltip).toBeVisible();
  await trigger.press("Enter");
  const dialog = testPage.getByRole("dialog", { name: "Visual output error" });
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText("StatusList");
  await expect(report).toContainText(RICH_OUTPUT_FILE_CONTENT.trim());
  await expect(chartFilter).toHaveAttribute("aria-pressed", "false");
  await testPage.screenshot({ path: testInfo.outputPath("rich-output-error-desktop.png") });
  await prCapture.screenshot("desktop-error-popover", {
    caption: "Desktop rich-output error disclosure",
  });

  await testPage.keyboard.press("Escape");
  await expect(dialog).toBeHidden();
  await expect(trigger).toBeFocused();
  await expect(chartFilter).toHaveAttribute("aria-pressed", "false");
  await trigger.click();
  await expect(dialog).toBeVisible();
  await trigger.click();
  await expect(dialog).toBeHidden();
  await trigger.click();
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "Close", exact: true }).click();
  await expect(dialog).toBeHidden();
  await expect(trigger).toBeFocused();
  await trigger.press("Space");
  await expect(dialog).toBeVisible();
  await testPage.keyboard.press("Escape");

  expect(await testPage.evaluate(() => window.matchMedia("(pointer: fine)").matches)).toBe(true);
  await testPage.setViewportSize({ width: 767, height: 844 });
  const narrowTrigger = chat.getByTestId("rich-output-error-trigger");
  await narrowTrigger.click();
  const drawer = testPage.getByTestId("rich-output-error-drawer");
  await expect(drawer).toBeVisible();
  await drawer.getByRole("button", { name: "Close", exact: true }).click();
  await testPage.setViewportSize({ width: 768, height: 844 });
  await narrowTrigger.click();
  await expect(testPage.getByTestId("rich-output-error-popover")).toBeVisible();
  await testPage.keyboard.press("Escape");
  await testPage.setViewportSize({ width: 1280, height: 900 });

  await testPage.reload();
  await session.waitForLoad();
  await expect(chat.getByTestId("rich-output-error-trigger")).toHaveCount(1);
  await expect(chat.getByTestId("rich-output")).toContainText("Corrected verification");
  await expect(
    chat.getByTestId("rich-output").getByTestId("rich-output-file-preview-toggle"),
  ).toHaveAttribute("aria-expanded", "false");
});
