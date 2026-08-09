import { test, expect } from "../../fixtures/test-base";

test.describe("Mobile config chat popover", () => {
  test("keeps the floating Save above config chat and inside a short viewport", async ({
    testPage,
    apiClient,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 667 });
    const initial = await apiClient.getUserSettings();
    const initialLayout = initial.settings.changes_panel_layout === "tree" ? "tree" : "flat";
    const nextLayout = initialLayout === "tree" ? "flat" : "tree";

    try {
      await testPage.goto("/settings/general/appearance");
      const layout = testPage.getByTestId("changes-panel-layout-select");
      await layout.tap();
      await testPage
        .getByRole("option", { name: nextLayout === "tree" ? "Tree" : "Flat list" })
        .tap();

      await testPage.getByRole("button", { name: "Configuration Chat" }).tap();
      const floatingSave = testPage.getByTestId("settings-floating-save");
      const saveButton = floatingSave.getByRole("button", { name: "Save changes" });
      const resetButton = floatingSave.getByRole("button", { name: "Reset" });
      const surface = floatingSave.getByTestId("settings-floating-save-surface");
      const configChatPopover = testPage.getByTestId("config-chat-popover");
      await expect(configChatPopover).toBeVisible();

      const [saveBox, chatBox, surfaceBox] = await Promise.all([
        saveButton.boundingBox(),
        configChatPopover.boundingBox(),
        surface.boundingBox(),
      ]);
      expect(saveBox).not.toBeNull();
      expect(chatBox).not.toBeNull();
      expect(surfaceBox).not.toBeNull();
      expect(saveBox!.height).toBeGreaterThanOrEqual(44);
      const resetBox = await resetButton.boundingBox();
      expect(resetBox).not.toBeNull();
      expect(resetBox!.height).toBeGreaterThanOrEqual(44);
      expect(saveBox!.y).toBeGreaterThanOrEqual(0);
      expect(saveBox!.y + saveBox!.height).toBeLessThanOrEqual(chatBox!.y);
      expect(saveBox!.x + saveBox!.width).toBeLessThanOrEqual(390);
      expect(surfaceBox!.x).toBeGreaterThanOrEqual(0);
      expect(surfaceBox!.x + surfaceBox!.width).toBeLessThanOrEqual(390);
      expect(
        Math.abs(surfaceBox!.x + surfaceBox!.width / 2 - (chatBox!.x + chatBox!.width / 2)),
      ).toBeLessThanOrEqual(2);
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
      ).toBe(false);

      await saveButton.tap();
      await expect(testPage.getByTestId("settings-floating-save")).not.toBeVisible();
      await expect(configChatPopover).toBeVisible();
    } finally {
      await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
        changes_panel_layout: initialLayout,
      });
    }
  });
});
