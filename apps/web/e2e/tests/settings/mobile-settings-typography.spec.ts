import { test, expect } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";

async function expectContained(page: Page) {
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    await page.evaluate(() => document.documentElement.clientWidth),
  );
}

test.describe("Settings typography contract on mobile", () => {
  test("keeps navigation rows and controls touch-sized on a phone", async ({
    testPage,
    prCapture,
  }) => {
    await testPage.goto("/settings");

    const index = testPage.getByTestId("settings-index");
    const appearanceLink = index.getByRole("link", { name: "Appearance", exact: true });
    await expect(appearanceLink).toBeVisible();
    const linkBox = await appearanceLink.boundingBox();
    expect(linkBox).not.toBeNull();
    expect(linkBox!.height).toBeGreaterThanOrEqual(44);
    await expectContained(testPage);

    await appearanceLink.click();
    await expect(testPage).toHaveURL(/\/settings\/preferences\/appearance$/);
    await expect(testPage.getByRole("heading", { level: 2 })).toBeVisible();
    const controlBox = await testPage
      .getByTestId("theme-settings-card")
      .locator("button")
      .boundingBox();
    expect(controlBox).not.toBeNull();
    expect(controlBox!.height).toBeGreaterThanOrEqual(44);
    await expectContained(testPage);

    await prCapture.screenshot("appearance-mobile-typography", {
      caption: "Settings Appearance page with touch-sized mobile controls",
    });
  });

  test("keeps the md breakpoint composition contained at tablet width", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 700, height: 900 });
    await testPage.goto("/settings/preferences/appearance");

    await expect(testPage.getByRole("heading", { level: 2 })).toBeVisible();
    const section = testPage.getByRole("heading", { level: 3, name: "Appearance", exact: true });
    await expect(section).toBeVisible();
    const controlBox = await testPage
      .getByTestId("theme-settings-card")
      .locator("button")
      .boundingBox();
    expect(controlBox).not.toBeNull();
    expect(controlBox!.height).toBeGreaterThanOrEqual(44);
    await expectContained(testPage);
  });
});
