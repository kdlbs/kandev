import { test, expect } from "../../fixtures/test-base";

test.describe("Settings typography contract on desktop", () => {
  test("keeps page, section, and card headings distinct", async ({ testPage, prCapture }) => {
    await testPage.setViewportSize({ width: 1280, height: 900 });
    await testPage.goto("/settings/preferences/appearance");

    const pageTitle = testPage.getByRole("heading", { level: 2, name: "Appearance", exact: true });
    const sectionTitle = testPage.getByRole("heading", {
      level: 3,
      name: "Appearance",
      exact: true,
    });
    const card = testPage.getByTestId("theme-settings-card");
    await expect(pageTitle).toBeVisible();
    await expect(sectionTitle).toBeVisible();
    await expect(card.locator("h3")).toHaveCount(1);

    const pageSize = await pageTitle.evaluate((element) =>
      parseFloat(getComputedStyle(element).fontSize),
    );
    const sectionSize = await sectionTitle.evaluate((element) =>
      parseFloat(getComputedStyle(element).fontSize),
    );
    const cardSize = await card
      .locator("h3")
      .evaluate((element) => parseFloat(getComputedStyle(element).fontSize));
    expect(pageSize).toBeCloseTo(24, 1);
    expect(sectionSize).toBeCloseTo(18, 1);
    expect(cardSize).toBeCloseTo(16, 1);
    await expect(testPage.getByTestId("theme-settings-card").locator("button")).toHaveCount(1);
    const controlBox = await testPage
      .getByTestId("theme-settings-card")
      .locator("button")
      .boundingBox();
    expect(controlBox).not.toBeNull();
    expect(controlBox!.height).toBeGreaterThanOrEqual(28);
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
      await testPage.evaluate(() => document.documentElement.clientWidth),
    );

    await prCapture.screenshot("appearance-desktop-typography", {
      caption: "Settings Appearance page with shared desktop typography roles",
    });
  });

  test("uses one page heading on the combined Terminal and Editors route", async ({
    testPage,
    prCapture,
  }) => {
    await testPage.setViewportSize({ width: 1280, height: 900 });
    await testPage.goto("/settings/preferences/terminal-editors");

    await expect(testPage.getByTestId("terminal-font-select")).toBeVisible();
    await expect(testPage.getByRole("heading", { level: 2 })).toHaveCount(1);
    await expect(testPage.getByRole("heading", { level: 3, name: "Editors" })).toBeVisible();
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
      await testPage.evaluate(() => document.documentElement.clientWidth),
    );

    await prCapture.screenshot("terminal-editors-desktop-typography", {
      caption: "Combined Terminal and Editors settings page",
    });
  });
});
