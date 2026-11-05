import { test, expect } from "../../fixtures/test-base";
import { exerciseShortcutFilter } from "./shortcut-filter-helpers";

// @covers AC-UI-SHORTCUT-SETTINGS-FILTER-001.1, .2, .3, .4, .5
test("filters shortcut settings with reachable phone controls", async ({ testPage, prCapture }) => {
  const { search, clear, github } = await exerciseShortcutFilter(testPage);
  for (const control of [search, clear]) {
    const box = await control.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.height).toBeGreaterThanOrEqual(44);
    expect(box!.x).toBeGreaterThanOrEqual(0);
    expect(box!.x + box!.width).toBeLessThanOrEqual(testPage.viewportSize()!.width);
  }
  const clearBox = await clear.boundingBox();
  expect(clearBox!.width).toBeGreaterThanOrEqual(44);
  expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(
    await testPage.evaluate(() => document.documentElement.clientWidth),
  );
  await testPage.getByTestId("settings-scroll-container").evaluate((element) => {
    element.scrollBy(0, 150);
  });
  await expect(search).toBeVisible();
  await expect(github).toBeVisible();
  const rowBox = await github.boundingBox();
  const saveBox = await testPage.getByTestId("settings-floating-save-surface").boundingBox();
  expect(rowBox!.y + rowBox!.height).toBeLessThanOrEqual(saveBox!.y);
  await prCapture.screenshot("keyboard-shortcut-filter-phone", {
    caption: "Phone shortcut settings filtered to GitHub",
  });
});
