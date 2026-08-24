import { test, expect } from "../../fixtures/test-base";
import type { Locator, Page } from "@playwright/test";
import { openWideTask, resizeColumnViaSplitview } from "../../helpers/dockview-resize";

/**
 * A group narrow enough to squeeze its tab row has to take the space out of
 * the titles, not out of the close buttons.
 *
 * Dockview pins `.dv-tab` at `flex-shrink: 0`, so before the theme override in
 * `app/dockview-theme.css` the titles stayed at full width and the strip
 * clipped the row from the right instead. The close button sits at a tab's
 * right edge, so it was the first thing to go and a tab could become
 * impossible to close without widening the group again.
 */

type TabProbe = {
  title: string;
  tabWidth: number;
  closeWidth: number;
  truncated: boolean;
};

/**
 * Locate the Dockview group that owns the `Files` tab and hand back the two
 * elements every reading here comes from.
 *
 * Dockview can mount more than one group, so the lookup refuses to guess:
 * measuring one tab row while scrolling another would still look like a pass.
 * Returned via `data-*` markers because `page.evaluate` cannot return DOM
 * handles to the test process.
 */
async function markFilesGroup(page: Page): Promise<{ strip: Locator; row: Locator }> {
  await page.evaluate(() => {
    const strips = Array.from(document.querySelectorAll(".dv-tabs-and-actions-container"));
    const matches = strips.filter((strip) =>
      Array.from(strip.querySelectorAll(".dv-default-tab-content")).some(
        (label) => label.textContent?.trim() === "Files",
      ),
    );
    if (matches.length !== 1) {
      throw new Error(
        `expected exactly one tab strip holding a Files tab, found ${matches.length}`,
      );
    }
    for (const marked of Array.from(document.querySelectorAll("[data-probe-strip]"))) {
      marked.removeAttribute("data-probe-strip");
    }
    matches[0].setAttribute("data-probe-strip", "files");
  });
  const strip = page.locator("[data-probe-strip='files']");
  return { strip, row: strip.locator(".dv-tabs-container") };
}

/** Measure every tab of the marked group. */
async function probeTabStrip(strip: Locator): Promise<TabProbe[]> {
  return strip.evaluate((element) =>
    Array.from(element.querySelectorAll(".dv-tab")).map((tab) => {
      const label = tab.querySelector(".dv-default-tab-content") as HTMLElement | null;
      const close = tab.querySelector(".dv-default-tab-action") as HTMLElement | null;
      if (!label || !close) {
        throw new Error(
          `tab "${label?.textContent?.trim() ?? tab.className}" has no label or close button`,
        );
      }
      return {
        title: label.textContent?.trim() ?? "",
        tabWidth: tab.getBoundingClientRect().width,
        closeWidth: close.getBoundingClientRect().width,
        truncated: label.scrollWidth > label.clientWidth,
      };
    }),
  );
}

const rowWidth = (tabs: TabProbe[]): number => tabs.reduce((total, tab) => total + tab.tabWidth, 0);

const rowOverflow = (row: Locator): Promise<number> =>
  row.evaluate((element) => element.scrollWidth - element.clientWidth);

const rowScrollLeft = (row: Locator): Promise<number> =>
  row.evaluate((element) => element.scrollLeft);

test.describe("narrow tab strip", () => {
  test("squeezes tab titles rather than the close buttons", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await openWideTask(testPage, apiClient, seedData, "Narrow right column tabs");
    const { strip } = await markFilesGroup(testPage);

    const wide = await probeTabStrip(strip);
    expect(wide.length).toBeGreaterThan(1);
    expect(wide.map((tab) => tab.truncated)).not.toContain(true);
    expect(wide.every((tab) => tab.closeWidth > 0)).toBe(true);

    await resizeColumnViaSplitview(testPage, "right", 240);
    const narrow = await probeTabStrip(strip);

    expect(narrow.map((tab) => tab.title)).toEqual(wide.map((tab) => tab.title));
    // The tabs give up width...
    expect(rowWidth(narrow)).toBeLessThan(rowWidth(wide));
    // ...their titles ellipsize...
    expect(narrow.map((tab) => tab.truncated)).toContain(true);
    // ...and the close buttons keep every pixel they had.
    expect(narrow.map((tab) => tab.closeWidth)).toEqual(wide.map((tab) => tab.closeWidth));
  });

  test("scrolls the tab row with a horizontal wheel gesture", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await openWideTask(testPage, apiClient, seedData, "Horizontal tab scroll");
    await resizeColumnViaSplitview(testPage, "right", 240);
    const { row } = await markFilesGroup(testPage);

    const filesTab = row.locator(".dv-default-tab").filter({ hasText: /^Files$/ });
    await expect(filesTab).toBeVisible();
    expect(await rowOverflow(row)).toBeGreaterThan(0);

    // A two-finger horizontal touchpad swipe reaches the page as `deltaX`,
    // which Dockview's own wheel handler ignores.
    await filesTab.hover();
    await testPage.mouse.wheel(60, 0);

    await expect.poll(() => rowScrollLeft(row)).toBeGreaterThan(0);
  });
});
