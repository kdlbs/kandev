import { test, expect } from "../../fixtures/test-base";

const APPEARANCE_PATH = "/settings/general/appearance";
const TERMINAL_PATH = "/settings/general/terminal";

test.describe("Settings manual save", () => {
  test("keeps Appearance changes local and guards dirty navigation", async ({
    testPage,
    apiClient,
  }) => {
    const initial = await apiClient.getUserSettings();
    const initialLayout = initial.settings.changes_panel_layout === "tree" ? "tree" : "flat";
    const nextLayout = initialLayout === "tree" ? "flat" : "tree";

    try {
      await testPage.goto(APPEARANCE_PATH);
      await expect(
        testPage.getByRole("heading", { name: "Appearance", exact: true }),
      ).toBeVisible();

      const layout = testPage.getByTestId("changes-panel-layout-select");
      await layout.click();
      await testPage
        .getByRole("option", { name: nextLayout === "tree" ? "Tree" : "Flat list" })
        .click();

      const floatingSave = testPage.getByTestId("settings-floating-save");
      await expect(floatingSave).toBeVisible();
      expect((await apiClient.getUserSettings()).settings.changes_panel_layout).toBe(
        initial.settings.changes_panel_layout,
      );

      await testPage.getByRole("link", { name: "Terminal", exact: true }).first().click();
      const navigationDialog = testPage.getByRole("alertdialog", {
        name: "Save changes before leaving?",
      });
      await expect(navigationDialog).toBeVisible();
      await navigationDialog.getByRole("button", { name: "Continue editing" }).click();
      await expect(testPage).toHaveURL(new RegExp(`${APPEARANCE_PATH}$`));

      await testPage.getByRole("link", { name: "Terminal", exact: true }).first().click();
      await expect(navigationDialog).toBeVisible();
      await navigationDialog.getByRole("button", { name: "Save and leave" }).click();
      await expect(testPage).toHaveURL(new RegExp(`${TERMINAL_PATH}$`));
      expect((await apiClient.getUserSettings()).settings.changes_panel_layout).toBe(nextLayout);
    } finally {
      await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
        changes_panel_layout: initialLayout,
      });
    }
  });

  test("persists Terminal changes only when the floating action is pressed", async ({
    testPage,
    apiClient,
  }) => {
    const initial = await apiClient.getUserSettings();
    const initialSize = Number(initial.settings.terminal_font_size) || 13;
    const nextSize = initialSize === 18 ? 17 : 18;

    try {
      await testPage.goto(TERMINAL_PATH);
      const sizeInput = testPage.getByTestId("terminal-font-size-input");
      await expect(sizeInput).toBeVisible();
      await sizeInput.fill(String(nextSize));

      expect((await apiClient.getUserSettings()).settings.terminal_font_size).toBe(
        initial.settings.terminal_font_size,
      );
      const floatingSave = testPage.getByTestId("settings-floating-save");
      await expect(floatingSave).toBeVisible();
      await floatingSave.getByRole("button", { name: "Save changes" }).click();
      await expect(floatingSave).not.toBeVisible({ timeout: 15_000 });
      expect((await apiClient.getUserSettings()).settings.terminal_font_size).toBe(nextSize);

      await testPage.reload();
      await expect(testPage.getByTestId("terminal-font-size-input")).toHaveValue(String(nextSize));
    } finally {
      await apiClient.saveUserSettings({ terminal_font_size: initialSize });
    }
  });
});
