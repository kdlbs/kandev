import { test, expect } from "../../fixtures/test-base";
import { resizeColumnViaSplitview } from "../../helpers/dockview-resize";
import { checkDownload, checkGeometry, openDownloadPlan } from "./plan-download-helpers";

test("downloads the current Plan draft on a phone", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  await openDownloadPlan(testPage, apiClient, seedData, true);
  await checkGeometry(testPage, true);
  await prCapture.screenshot("plan-download-phone", {
    caption: "Download the current Plan draft on a phone.",
  });
  await checkDownload(testPage, true);
});

test("keeps the download control touch-sized on a coarse-pointer workbench", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await testPage.setViewportSize({ width: 1280, height: 900 });
  const panel = await openDownloadPlan(testPage, apiClient, seedData, false);
  await checkGeometry(testPage, true);
  expect(await resizeColumnViaSplitview(testPage, "right", 180)).toBe(180);
  await expect.poll(async () => Math.round((await panel.boundingBox())?.width ?? 0)).toBe(180);
  const download = panel.getByRole("button", { name: "Download", exact: true });
  const implement = panel.getByTestId("plan-toolbar-implement-button");
  const menu = panel.getByTestId("plan-toolbar-implement-menu-trigger");
  const history = panel.getByTestId("plan-rewind-button");
  for (const control of [implement, menu, download, history]) {
    const controlBox = await control.boundingBox();
    expect(controlBox!.width).toBeGreaterThanOrEqual(44);
    expect(controlBox!.height).toBeGreaterThanOrEqual(44);
  }
  expect(
    await implement.evaluate((element) => {
      const rect = element.getBoundingClientRect();
      const header = element.closest(".border-b")!.getBoundingClientRect();
      const hit = document.elementFromPoint(rect.left + rect.width / 2, rect.top + rect.height / 2);
      return (
        rect.left >= header.left &&
        rect.right <= header.right &&
        (hit === element || element.contains(hit))
      );
    }),
  ).toBe(true);
  await menu.tap();
  await expect(testPage.getByTestId("plan-toolbar-implement-fresh-menu-item")).toBeVisible();
  await testPage.keyboard.press("Escape");
  await expect(menu).toBeFocused();
  await expect(testPage.getByTestId("plan-toolbar-implement-fresh-menu-item")).toBeHidden();
  const actions = download.locator("xpath=..");
  const box = await actions.boundingBox();
  expect(box).not.toBeNull();
  const cdp = await testPage.context().newCDPSession(testPage);
  const y = box!.y + box!.height / 2;
  const startX = box!.x + 40;
  await cdp.send("Input.dispatchTouchEvent", {
    type: "touchStart",
    touchPoints: [{ x: startX, y }],
  });
  for (let step = 1; step <= 5; step++) {
    await cdp.send("Input.dispatchTouchEvent", {
      type: "touchMove",
      touchPoints: [{ x: startX - step * 20, y }],
    });
  }
  await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
  await cdp.detach();
  await expect.poll(() => actions.evaluate((element) => element.scrollLeft)).toBeGreaterThan(0);
  await expect(history).toBeEnabled();
  await history.tap();
  await expect(testPage.getByTestId("plan-revisions-popover")).toBeVisible();
  await testPage.keyboard.press("Escape");
  await checkDownload(testPage, true);
  expect(
    await testPage.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
});
