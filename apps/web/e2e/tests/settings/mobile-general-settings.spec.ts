import { test, expect } from "../../fixtures/test-base";

test.describe("Mobile general settings", () => {
  test("opens a dedicated General settings page from the overview", async ({ testPage }) => {
    await testPage.goto("/settings/general");

    await expect(testPage.getByRole("link", { name: /Terminal/ })).toBeVisible({
      timeout: 15_000,
    });

    await testPage.getByRole("link", { name: /Terminal/ }).click();

    await expect(testPage).toHaveURL(/\/settings\/general\/terminal$/);
    await expect(testPage.getByRole("heading", { name: "Terminal", exact: true })).toBeVisible();
    await expect(testPage.getByTestId("terminal-font-select")).toBeVisible();
    await expect(testPage.getByTestId("terminal-font-size-input")).toBeVisible();
  });

  test("opens Settings navigation and returns home from a nested settings page", async ({
    testPage,
  }) => {
    await testPage.goto("/settings/general/terminal");

    await expect(testPage.getByRole("heading", { name: "Terminal", exact: true })).toBeVisible();

    await testPage.getByTestId("settings-mobile-menu-button").click();
    const menu = testPage.getByTestId("settings-mobile-menu");
    await expect(menu).toBeVisible();

    await menu.getByRole("link", { name: "Appearance" }).click();

    await expect(testPage).toHaveURL(/\/settings\/general\/appearance$/);
    await expect(menu).not.toBeVisible();
    await expect(testPage.getByRole("heading", { name: "Appearance", exact: true })).toBeVisible();

    await testPage.getByTestId("settings-mobile-menu-button").click();
    await testPage.getByTestId("settings-mobile-menu").getByRole("link", { name: "Home" }).click();

    await expect(testPage).toHaveURL(/\/(?:\?.*)?$/);
    await expect(testPage.getByTestId("kanban-board")).toBeVisible();
  });
});
