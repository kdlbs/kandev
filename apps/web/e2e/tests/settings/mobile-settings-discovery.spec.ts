import type { Locator } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";

function settingsResult(scope: Locator, label: string) {
  return scope.getByRole("link").filter({ hasText: new RegExp(`^${label}`) });
}

test.describe("Mobile settings discovery", () => {
  test("searches in the Settings sheet and reveals the exact control", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto("/settings/general/appearance");

    const menuButton = testPage.getByTestId("settings-mobile-menu-button");
    const menuButtonBox = await menuButton.boundingBox();
    expect(menuButtonBox).not.toBeNull();
    expect(menuButtonBox!.width).toBeGreaterThanOrEqual(44);
    expect(menuButtonBox!.height).toBeGreaterThanOrEqual(44);
    await menuButton.click();

    const menu = testPage.getByTestId("settings-mobile-menu");
    await expect(menu).toBeVisible();
    await expect
      .poll(async () => (await menu.boundingBox())?.x ?? -1, { message: "Settings sheet settled" })
      .toBeGreaterThanOrEqual(0);
    const menuBox = await menu.boundingBox();
    expect(menuBox).not.toBeNull();
    expect(menuBox!.x).toBeGreaterThanOrEqual(0);
    expect(menuBox!.x + menuBox!.width).toBeLessThanOrEqual(390);

    const navigation = menu.locator("nav");
    expect(await navigation.evaluate((element) => getComputedStyle(element).overflowY)).toBe(
      "auto",
    );
    expect(
      await navigation.locator("*").evaluateAll(
        (elements) =>
          elements.filter((element) => {
            const overflow = getComputedStyle(element).overflowY;
            return overflow === "auto" || overflow === "scroll";
          }).length,
      ),
    ).toBe(0);

    const search = menu.getByRole("searchbox", { name: "Search settings" });
    await search.fill("terminal font size");
    const searchBox = await search.boundingBox();
    expect(searchBox).not.toBeNull();
    expect(searchBox!.height).toBeGreaterThanOrEqual(44);

    const clear = menu.getByRole("button", { name: "Clear settings search" });
    const clearBox = await clear.boundingBox();
    expect(clearBox).not.toBeNull();
    expect(clearBox!.width).toBeGreaterThanOrEqual(44);
    expect(clearBox!.height).toBeGreaterThanOrEqual(44);

    const result = settingsResult(menu, "Terminal Font Size");
    await expect(result).toBeVisible();
    const resultBox = await result.boundingBox();
    expect(resultBox).not.toBeNull();
    expect(resultBox!.height).toBeGreaterThanOrEqual(44);
    await result.click();

    await expect(menu).not.toBeVisible();
    await expect(testPage).toHaveURL(/\/settings\/general\/terminal#setting-terminal-font-size$/);
    const target = testPage.locator('[data-settings-target="setting-terminal-font-size"]');
    await expect(target).toHaveAttribute("data-settings-target-highlight", "true");
    await expect(testPage.getByTestId("terminal-font-size-input")).toBeFocused();
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
      await testPage.evaluate(() => document.documentElement.clientWidth),
    );
    expect(
      await testPage
        .getByTestId("settings-scroll-container")
        .evaluate((element) => getComputedStyle(element).overflowY),
    ).toBe("auto");
  });
});
