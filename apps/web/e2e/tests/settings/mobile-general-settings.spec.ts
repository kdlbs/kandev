import { test, expect } from "../../fixtures/test-base";

test.describe("Mobile general settings", () => {
  test("keeps the floating Save reachable without covering the last control", async ({
    testPage,
    apiClient,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    const initial = await apiClient.getUserSettings();
    const initialLayout = initial.settings.changes_panel_layout === "tree" ? "tree" : "flat";
    const nextLayout = initialLayout === "tree" ? "flat" : "tree";

    try {
      await testPage.goto("/settings/general/appearance");
      const layout = testPage.getByTestId("changes-panel-layout-select");
      await layout.click();
      await testPage
        .getByRole("option", { name: nextLayout === "tree" ? "Tree" : "Flat list" })
        .click();

      const floating = testPage.getByTestId("settings-floating-save");
      const saveButton = floating.getByRole("button", { name: "Save changes" });
      await expect(saveButton).toBeVisible();
      await expect(layout).toHaveAttribute("data-settings-dirty", "true");
      await expect(testPage.getByTestId("changes-panel-layout-card")).toHaveAttribute(
        "data-settings-dirty",
        "true",
      );
      const saveBox = await saveButton.boundingBox();
      expect(saveBox).not.toBeNull();
      expect(saveBox!.height).toBeGreaterThanOrEqual(44);
      expect(saveBox!.x + saveBox!.width).toBeLessThanOrEqual(390 - 16 + 1);
      expect(saveBox!.y + saveBox!.height).toBeLessThanOrEqual(844 - 16 + 1);

      const lastControl = testPage.locator("#metrics-disk-path");
      await lastControl.scrollIntoViewIfNeeded();
      const lastControlBox = await lastControl.boundingBox();
      expect(lastControlBox).not.toBeNull();
      expect(lastControlBox!.y + lastControlBox!.height).toBeLessThanOrEqual(saveBox!.y);
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
      ).toBe(false);

      await saveButton.click();
      await expect(floating).not.toBeVisible({ timeout: 15_000 });
      await expect(layout).toHaveAttribute("data-settings-dirty", "false");
      await expect(testPage.getByTestId("changes-panel-layout-card")).toHaveAttribute(
        "data-settings-dirty",
        "false",
      );
    } finally {
      await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
        changes_panel_layout: initialLayout,
      });
    }
  });

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
