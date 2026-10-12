import { test, expect } from "../../fixtures/test-base";
import {
  INTEGRATION_CHORD,
  INTEGRATION_SHORTCUTS_SETTINGS_PATH,
  recordIntegrationShortcut,
  saveIntegrationShortcuts,
} from "./integration-shortcuts-helpers";

// @covers AC-UI-INTEGRATION-PAGE-SHORTCUTS-001.3, .4, .7, .8
test("configures integration hotkeys through phone settings and keeps controls contained", async ({
  testPage,
  apiClient,
  prCapture,
}, testInfo) => {
  const baseline =
    ((await apiClient.getUserSettings()).settings.keyboard_shortcuts as Record<string, unknown>) ??
    {};
  try {
    await apiClient.saveUserSettings({ keyboard_shortcuts: {} });
    await testPage.goto("/settings");
    const index = testPage.getByTestId("settings-index");
    const preferences = index.getByRole("button", { name: "Expand Preferences" });
    if (await preferences.isVisible()) await preferences.tap();
    await index.locator(`a[href='${INTEGRATION_SHORTCUTS_SETTINGS_PATH}']`).tap();
    const recorder = await recordIntegrationShortcut(testPage, "github", true);
    const originalViewport = testPage.viewportSize()!;
    await testPage.setViewportSize({
      width: originalViewport.height,
      height: originalViewport.width,
    });
    await expect(recorder).toContainText("G");
    await expect(recorder).toHaveAttribute("data-settings-dirty", "true");
    await testPage.setViewportSize(originalViewport);
    await recorder.scrollIntoViewIfNeeded();
    const box = await recorder.boundingBox();
    expect(box!.height).toBeGreaterThanOrEqual(44);
    const reset = recorder.locator("..").getByRole("button", { name: "Reset (clear shortcut)" });
    const resetBox = await reset.boundingBox();
    expect(resetBox!.height).toBeGreaterThanOrEqual(44);
    expect(resetBox!.width).toBeGreaterThanOrEqual(44);
    const group = testPage.getByTestId("integration-shortcuts-group");
    const groupBox = await group.boundingBox();
    expect(box!.x).toBeGreaterThanOrEqual(groupBox!.x);
    expect(resetBox!.x + resetBox!.width).toBeLessThanOrEqual(groupBox!.x + groupBox!.width);
    const lastRecorder = testPage.getByTestId("shortcut-recorder-integration:sentry");
    await lastRecorder.scrollIntoViewIfNeeded();
    const save = testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" });
    const [lastBox, saveBox] = await Promise.all([lastRecorder.boundingBox(), save.boundingBox()]);
    expect(lastBox!.y + lastBox!.height).toBeLessThanOrEqual(saveBox!.y);
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
      ),
    ).toBe(false);
    await testPage.screenshot({ path: testInfo.outputPath("integration-shortcuts-phone.png") });
    await expect(recorder).toHaveAccessibleDescription("Ctrl+Alt+G");
    await prCapture.screenshot("integration-keybindings", {
      caption: "Integration hotkeys on a phone with touch controls and Save",
    });
    await saveIntegrationShortcuts(testPage, true);
    await testPage.reload();
    await expect(recorder).toContainText("G");
    await testPage.goto("/");
    const navigationStatus = testPage.getByRole("button", {
      name: "Open navigation menu",
      exact: true,
    });
    await expect(navigationStatus).toBeVisible();
    await navigationStatus.focus();
    await testPage.keyboard.press(INTEGRATION_CHORD);
    await expect(testPage).toHaveURL(/\/github$/);
    await testPage.goto(INTEGRATION_SHORTCUTS_SETTINGS_PATH);
    await recorder.scrollIntoViewIfNeeded();
    await reset.tap();
    await saveIntegrationShortcuts(testPage, true);
    await testPage.reload();
    await expect(recorder).toContainText("Unbound");
  } finally {
    await apiClient.saveUserSettings({ keyboard_shortcuts: baseline });
  }
});
