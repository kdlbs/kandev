import type { Locator } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";

function settingsResult(scope: Locator, label: string) {
  return scope.getByRole("link").filter({ hasText: new RegExp(`^${label}`) });
}

test.describe("Mobile settings discovery", () => {
  test("searches from the settings index and reveals the exact control", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    // The index is the settings navigation on a phone; the search field floats
    // over it in thumb reach rather than living in a drawer.
    await testPage.goto("/settings");

    const index = testPage.getByTestId("settings-index");
    const scope = index.getByTestId("settings-search");
    await expect(scope).toBeVisible();
    const searchBoxBounds = await scope.boundingBox();
    expect(searchBoxBounds).not.toBeNull();
    expect(searchBoxBounds!.x).toBeGreaterThanOrEqual(0);
    expect(searchBoxBounds!.x + searchBoxBounds!.width).toBeLessThanOrEqual(390);

    const search = scope.getByRole("searchbox", { name: "Search settings" });
    await search.fill("terminal font size");
    const searchBox = await search.boundingBox();
    expect(searchBox).not.toBeNull();
    expect(searchBox!.height).toBeGreaterThanOrEqual(44);

    const clear = scope.getByRole("button", { name: "Clear settings search" });
    const clearBox = await clear.boundingBox();
    expect(clearBox).not.toBeNull();
    expect(clearBox!.width).toBeGreaterThanOrEqual(44);
    expect(clearBox!.height).toBeGreaterThanOrEqual(44);

    const result = settingsResult(
      index.getByTestId("settings-search-results"),
      "Terminal Font Size",
    );
    await expect(result).toBeVisible();
    const resultBox = await result.boundingBox();
    expect(resultBox).not.toBeNull();
    expect(resultBox!.height).toBeGreaterThanOrEqual(44);
    await result.click();

    await expect(testPage).toHaveURL(
      /\/settings\/preferences\/terminal-editors#setting-terminal-font-size$/,
    );
    const target = testPage.locator('[data-settings-target="setting-terminal-font-size"]');
    await expect(target).toHaveAttribute("data-settings-target-highlight", "true");
    await expect(testPage.getByTestId("terminal-font-size-input")).toBeFocused();
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
      await testPage.evaluate(() => document.documentElement.clientWidth),
    );
  });
});
