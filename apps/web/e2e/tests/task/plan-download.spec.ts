import { test, expect } from "../../fixtures/test-base";
import { resizeColumnViaSplitview } from "../../helpers/dockview-resize";
import { checkDownload, checkGeometry, openDownloadPlan } from "./plan-download-helpers";

test("downloads the current Plan draft on desktop", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  await openDownloadPlan(testPage, apiClient, seedData, false);
  await checkGeometry(testPage, false);
  await prCapture.screenshot("plan-download-desktop", {
    caption: "Download the current Plan draft on desktop.",
  });
  await checkDownload(testPage, false);
});

test("keeps the Plan actions contained on a narrow fine-pointer phone", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await testPage.setViewportSize({ width: 375, height: 812 });
  await openDownloadPlan(testPage, apiClient, seedData, true);
  await checkGeometry(testPage, true);
});

test("keeps every Plan action reachable at the narrow desktop panel width", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  await testPage.setViewportSize({ width: 1366, height: 900 });
  const panel = await openDownloadPlan(testPage, apiClient, seedData, false);
  expect(await resizeColumnViaSplitview(testPage, "right", 180)).toBe(180);
  await expect.poll(async () => Math.round((await panel.boundingBox())?.width ?? 0)).toBe(180);

  const implement = panel.getByTestId("plan-toolbar-implement-button");
  const menu = panel.getByTestId("plan-toolbar-implement-menu-trigger");
  const download = panel.getByRole("button", { name: "Download", exact: true });
  const history = panel.getByTestId("plan-rewind-button");
  const actions = download.locator("xpath=..");
  expect(await actions.evaluate((element) => element.scrollWidth > element.clientWidth)).toBe(true);
  await actions.hover();
  await testPage.mouse.wheel(200, 0);
  await expect.poll(() => actions.evaluate((element) => element.scrollLeft)).toBeGreaterThan(0);
  await testPage.mouse.wheel(-200, 0);
  await expect.poll(() => actions.evaluate((element) => element.scrollLeft)).toBe(0);
  await expect(implement).toBeEnabled();
  await expect(menu).toBeEnabled();
  await expect(history).toBeEnabled();
  await implement.focus();
  for (const control of [implement, menu, download, history]) {
    await expect(control).toBeFocused();
    const isReachable = await control.evaluate((element) => {
      const rect = element.getBoundingClientRect();
      const header = element.closest(".border-b")!.getBoundingClientRect();
      const center = document.elementFromPoint(
        rect.left + rect.width / 2,
        rect.top + rect.height / 2,
      );
      return (
        rect.left >= header.left &&
        rect.right <= header.right &&
        rect.top >= header.top &&
        rect.bottom <= header.bottom &&
        (center === element || element.contains(center))
      );
    });
    expect(isReachable).toBe(true);
    if (control === menu) {
      await testPage.keyboard.press("Enter");
      await expect(testPage.getByTestId("plan-toolbar-implement-fresh-menu-item")).toBeVisible();
      await testPage.keyboard.press("Escape");
      await expect(menu).toBeFocused();
    }
    if (control !== history) await testPage.keyboard.press("Tab");
  }
  await prCapture.screenshot("plan-download-narrow", {
    caption: "The narrow Plan header scrolls to keep its actions reachable.",
  });
  await testPage.keyboard.press("Enter");
  await expect(testPage.getByTestId("plan-revisions-popover")).toBeVisible();
  await testPage.keyboard.press("Escape");
  await checkDownload(testPage, false);
  expect(
    await testPage.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
});
