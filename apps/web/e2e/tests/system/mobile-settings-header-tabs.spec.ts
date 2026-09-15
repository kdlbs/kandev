import { test, expect } from "../../fixtures/test-base";

test.describe("Settings header tabs on phones", () => {
  for (const width of [767, 768]) {
    test(`keeps the tab strip usable at the ${width}px breakpoint`, async ({ testPage }) => {
      await testPage.setViewportSize({ width, height: 844 });
      await testPage.goto("/settings/system/data-storage?tab=logs");

      const tabList = testPage.getByRole("tablist", { name: "Data & Logs" });
      await expect(tabList).toBeVisible();
      for (const tab of await testPage.getByRole("tab").all()) {
        const box = await tab.boundingBox();
        expect(box).not.toBeNull();
        expect(box!.height).toBeGreaterThanOrEqual(44);
      }
      await expect(testPage.getByTestId("customize-diagnostic-bundle")).toBeVisible();
      await expect
        .poll(() =>
          testPage.evaluate(
            () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
          ),
        )
        .toBe(true);
    });
  }

  test("places tabs below the description and returns focus from touch help", async ({
    testPage,
    prCapture,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto("/settings/system/storage?tab=office-retention");

    const title = testPage.getByTestId("system-page-title");
    const tabList = testPage.getByRole("tablist", { name: "Storage" });
    const titleBox = await title.boundingBox();
    const tabListBox = await tabList.boundingBox();
    expect(titleBox).not.toBeNull();
    expect(tabListBox).not.toBeNull();
    expect(tabListBox!.y).toBeGreaterThan(titleBox!.y + titleBox!.height);

    const officeTab = testPage.getByRole("tab", { name: "Office retention", exact: true });
    const hostTab = testPage.getByRole("tab", { name: "Host", exact: true });
    for (const tab of [hostTab, officeTab]) {
      const box = await tab.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
    }
    await expect(testPage.getByTestId("retention-status-card")).toBeVisible();

    const help = testPage
      .getByRole("button", { name: "More information about Retention window (days)" })
      .first();
    const helpBox = await help.boundingBox();
    expect(helpBox).not.toBeNull();
    expect(helpBox!.width).toBeGreaterThanOrEqual(44);
    expect(helpBox!.height).toBeGreaterThanOrEqual(44);
    await help.tap();

    const drawer = testPage.getByRole("dialog");
    await expect(drawer).toBeVisible();
    await expect(drawer).toContainText("Finished rows older than this many days");
    await drawer.getByRole("button", { name: "Close" }).tap();
    await expect(help).toBeFocused();

    const windowField = testPage.getByTestId("retention-routine-runs-window-days");
    const floorField = testPage.getByTestId("retention-routine-runs-floor-per-owner");
    const windowBox = await windowField.boundingBox();
    const floorBox = await floorField.boundingBox();
    expect(windowBox).not.toBeNull();
    expect(floorBox).not.toBeNull();
    expect(floorBox!.y).toBeGreaterThan(windowBox!.y + windowBox!.height);
    await expect
      .poll(() =>
        testPage.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      )
      .toBe(true);
    if (prCapture.capturing) {
      await testPage.evaluate(() => {
        window.scrollTo(0, 0);
        for (const element of document.querySelectorAll<HTMLElement>("*")) {
          const style = getComputedStyle(element);
          if (
            (style.overflowY === "auto" || style.overflowY === "scroll") &&
            element.scrollHeight > element.clientHeight
          ) {
            element.scrollTop = 0;
          }
        }
      });
    }
    await prCapture.screenshot("mobile-storage-office-retention", {
      caption: "Office retention keeps status, policy, tabs, and touch help usable on a phone.",
      fullPage: true,
    });
  });
});
