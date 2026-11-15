import { test, expect } from "../../fixtures/test-base";
import { exerciseShortcutFilter } from "./shortcut-filter-helpers";

// @covers AC-UI-SHORTCUT-SETTINGS-FILTER-001.1, .2, .3, .4, .5
test("filters shortcut settings and preserves drafts on desktop", async ({
  testPage,
  prCapture,
}) => {
  const { search, clear } = await exerciseShortcutFilter(testPage);
  for (const control of [search, clear]) {
    const box = await control.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.height).toBeGreaterThanOrEqual(27);
    expect(box!.height).toBeLessThanOrEqual(29);
  }
  await prCapture.screenshot("keyboard-shortcut-filter-desktop", {
    caption: "Desktop shortcut settings filtered to GitHub",
  });
});

// @covers AC-UI-SHORTCUT-SETTINGS-FILTER-001.5
test("keeps filter controls reachable on a narrow fine-pointer viewport", async ({ testPage }) => {
  await testPage.setViewportSize({ width: 767, height: 900 });
  expect(await testPage.evaluate(() => matchMedia("(pointer: fine)").matches)).toBe(true);
  const { search, clear, github } = await exerciseShortcutFilter(testPage);
  for (const control of [search, clear]) {
    const box = await control.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.height).toBeGreaterThanOrEqual(44);
    expect(box!.x).toBeGreaterThanOrEqual(0);
    expect(box!.x + box!.width).toBeLessThanOrEqual(767);
  }
  expect((await clear.boundingBox())!.width).toBeGreaterThanOrEqual(44);
  expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(
    await testPage.evaluate(() => document.documentElement.clientWidth),
  );
  await clear.click();
  await expect(search).toHaveValue("");
  await expect(github).toBeVisible();
  await expect(testPage.getByTestId("shortcut-recorder-SEARCH")).toBeVisible();
});
