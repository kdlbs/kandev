import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

const APPEARANCE_PATH = "/settings/preferences/appearance";
const TERMINAL_PATH = "/settings/preferences/terminal-editors";
const NOTIFICATIONS_PATH = "/settings/preferences/notifications";
// The menu row the Terminal page now sits on; it was plain "Terminal" before
// the settings restructure merged Terminal and Editors into one page.
const TERMINAL_ROW = "Terminal & Editors";
const CLARIFICATION_REQUESTED = "session.clarification_requested";
const PROVIDER_NAME = "E2E semantic notifications";

type SeededProvider = {
  id: string;
};

async function seedNotificationProvider(
  apiClient: ApiClient,
  events: string[],
): Promise<SeededProvider> {
  const response = await apiClient.rawRequest("POST", "/api/v1/notification-providers", {
    name: PROVIDER_NAME,
    type: "local",
    events,
  });
  expect(response.ok).toBe(true);
  return (await response.json()) as SeededProvider;
}

test.describe("Settings manual save", () => {
  test("persists rich-output chart motion on this device only after Save", async ({ testPage }) => {
    await testPage.addInitScript(() => {
      if (window.localStorage.getItem("kandev.settings.richOutputAnimations") === null) {
        window.localStorage.setItem("kandev.settings.richOutputAnimations", "true");
      }
    });
    let userSettingsPatches = 0;
    testPage.on("request", (request) => {
      if (
        request.method() === "PATCH" &&
        new URL(request.url()).pathname === "/api/v1/user/settings"
      ) {
        userSettingsPatches += 1;
      }
    });
    await testPage.goto(APPEARANCE_PATH);

    const toggle = testPage.getByRole("switch", { name: "Animate rich-output charts" });
    await expect(toggle).toBeChecked();
    const toggleBox = await toggle.boundingBox();
    expect(toggleBox?.width).toBeGreaterThanOrEqual(44);
    expect(toggleBox?.height).toBeGreaterThanOrEqual(44);

    await toggle.click();
    await expect(toggle).not.toBeChecked();
    expect(
      await testPage.evaluate(() =>
        JSON.parse(window.localStorage.getItem("kandev.settings.richOutputAnimations") ?? "null"),
      ),
    ).toBe(true);

    const floatingSave = testPage.getByTestId("settings-floating-save");
    await floatingSave.getByRole("button", { name: "Save changes" }).click();
    await expect(floatingSave).not.toBeVisible();
    expect(userSettingsPatches).toBe(0);
    expect(
      await testPage.evaluate(() =>
        JSON.parse(window.localStorage.getItem("kandev.settings.richOutputAnimations") ?? "null"),
      ),
    ).toBe(false);

    await testPage.reload();
    await expect(toggle).not.toBeChecked();
  });

  test("keeps Appearance changes local and guards dirty navigation", async ({
    testPage,
    apiClient,
  }) => {
    const initial = await apiClient.getUserSettings();
    const initialLayout = initial.settings.changes_panel_layout === "tree" ? "tree" : "flat";
    const initialStatusBarEnabled = initial.settings.app_status_bar_enabled === true;
    const nextLayout = initialLayout === "tree" ? "flat" : "tree";

    try {
      await apiClient.saveUserSettings({ app_status_bar_enabled: true });
      await testPage.goto(APPEARANCE_PATH);
      await expect(
        testPage.getByRole("heading", { level: 2, name: "Appearance", exact: true }),
      ).toBeVisible();

      const layout = testPage.getByTestId("changes-panel-layout-select");
      await layout.click();
      await testPage
        .getByRole("option", { name: nextLayout === "tree" ? "Tree" : "Flat list" })
        .click();

      const floatingSave = testPage.getByTestId("settings-floating-save");
      await expect(floatingSave).toBeVisible();
      await expect(testPage.getByTestId("changes-panel-layout-card")).toHaveAttribute(
        "data-settings-dirty",
        "true",
      );
      expect((await apiClient.getUserSettings()).settings.changes_panel_layout).toBe(
        initial.settings.changes_panel_layout,
      );

      const surface = floatingSave.getByTestId("settings-floating-save-surface");
      const contentArea = testPage.getByTestId("settings-scroll-container");
      const configChatButton = testPage.getByRole("button", { name: "Configuration Chat" });
      const [surfaceBox, contentBox, configChatBox] = await Promise.all([
        surface.boundingBox(),
        contentArea.boundingBox(),
        configChatButton.boundingBox(),
      ]);
      expect(surfaceBox).not.toBeNull();
      expect(contentBox).not.toBeNull();
      expect(configChatBox).not.toBeNull();
      expect(surfaceBox!.height).toBeLessThanOrEqual(48);
      expect(
        Math.abs(surfaceBox!.x + surfaceBox!.width / 2 - (contentBox!.x + contentBox!.width / 2)),
      ).toBeLessThanOrEqual(2);
      expect(
        Math.abs(
          surfaceBox!.y + surfaceBox!.height / 2 - (configChatBox!.y + configChatBox!.height / 2),
        ),
      ).toBeLessThanOrEqual(2);
      await expect(floatingSave).not.toHaveClass(/bg-success/);
      await expect(floatingSave.getByRole("button", { name: "Save changes" })).toHaveClass(
        /bg-success/,
      );

      await floatingSave.getByRole("button", { name: "Reset" }).click();
      await expect(floatingSave).not.toBeVisible();
      await expect(testPage.getByTestId("changes-panel-layout-card")).toHaveAttribute(
        "data-settings-dirty",
        "false",
      );
      expect((await apiClient.getUserSettings()).settings.changes_panel_layout).toBe(
        initial.settings.changes_panel_layout,
      );

      await layout.click();
      await testPage
        .getByRole("option", { name: nextLayout === "tree" ? "Tree" : "Flat list" })
        .click();

      await testPage.getByRole("link", { name: TERMINAL_ROW, exact: true }).first().click();
      const navigationDialog = testPage.getByRole("alertdialog", {
        name: "Save changes before leaving?",
      });
      await expect(navigationDialog).toBeVisible();
      await navigationDialog.getByRole("button", { name: "Continue editing" }).click();
      await expect(testPage).toHaveURL(new RegExp(`${APPEARANCE_PATH}$`));

      await testPage.getByRole("link", { name: TERMINAL_ROW, exact: true }).first().click();
      await expect(navigationDialog).toBeVisible();
      await navigationDialog.getByRole("button", { name: "Save and leave" }).click();
      await expect(testPage).toHaveURL(new RegExp(`${TERMINAL_PATH}$`));
      expect((await apiClient.getUserSettings()).settings.changes_panel_layout).toBe(nextLayout);
    } finally {
      await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
        app_status_bar_enabled: initialStatusBarEnabled,
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
      await expect(testPage.getByTestId("terminal-font-size-card")).toHaveAttribute(
        "data-settings-dirty",
        "true",
      );
      await floatingSave.getByRole("button", { name: "Save changes" }).click();
      await expect(floatingSave).not.toBeVisible({ timeout: 15_000 });
      expect((await apiClient.getUserSettings()).settings.terminal_font_size).toBe(nextSize);

      await testPage.reload();
      await expect(testPage.getByTestId("terminal-font-size-input")).toHaveValue(String(nextSize));
    } finally {
      await apiClient.saveUserSettings({ terminal_font_size: initialSize });
    }
  });

  test("persists transcript navigation choices only when Save changes is pressed", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    const initial = await apiClient.getUserSettings();
    const initialAutoScrollControl = initial.settings.show_transcript_auto_scroll_control;

    try {
      await testPage.goto("/settings/preferences/task-behavior");
      const autoScrollControl = testPage.getByRole("switch", {
        name: "Show transcript auto-scroll control",
      });
      await expect(autoScrollControl).toBeChecked();
      await autoScrollControl.click();

      await prCapture.screenshot("transcript-navigation-draft", {
        caption: "Transcript Navigation settings with the Save action visible",
      });

      expect((await apiClient.getUserSettings()).settings.show_transcript_auto_scroll_control).toBe(
        true,
      );
      const floatingSave = testPage.getByTestId("settings-floating-save");
      await floatingSave.getByRole("button", { name: "Save changes" }).click();
      await expect(floatingSave).not.toBeVisible({ timeout: 15_000 });
      expect((await apiClient.getUserSettings()).settings.show_transcript_auto_scroll_control).toBe(
        false,
      );

      await testPage.reload();
      await expect(autoScrollControl).not.toBeChecked();
    } finally {
      await apiClient.saveUserSettings({
        show_transcript_auto_scroll_control: initialAutoScrollControl,
      });
    }
  });

  test("loads seeded semantic events and persists one independently selected event", async ({
    testPage,
    apiClient,
  }) => {
    const provider = await seedNotificationProvider(apiClient, [CLARIFICATION_REQUESTED]);

    try {
      await testPage.goto(NOTIFICATIONS_PATH);

      const turnFinished = testPage.getByRole("checkbox", {
        name: `Agent turn finished for ${PROVIDER_NAME}`,
      });
      const needsAnswer = testPage.getByRole("checkbox", {
        name: `Agent needs an answer for ${PROVIDER_NAME}`,
      });
      await expect(turnFinished).toBeVisible();
      await expect(needsAnswer).toBeVisible();
      await expect(turnFinished).not.toBeChecked();
      await expect(needsAnswer).toBeChecked();

      await turnFinished.click();
      await testPage
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: "Save changes" })
        .click();
      await expect(testPage.getByTestId("settings-floating-save")).not.toBeVisible({
        timeout: 15_000,
      });

      await testPage.reload();
      await expect(turnFinished).toBeChecked();
      await expect(needsAnswer).toBeChecked();
    } finally {
      await apiClient.rawRequest("DELETE", `/api/v1/notification-providers/${provider.id}`);
    }
  });

  test("keeps notification sound changes local until Save", async ({ testPage }) => {
    await testPage.addInitScript(() => {
      window.localStorage.setItem(
        "kandev.notifications.sound",
        JSON.stringify({ enabled: false, presetId: "plim" }),
      );
    });
    await testPage.goto(NOTIFICATIONS_PATH);

    const soundToggle = testPage.getByRole("switch", { name: "Enable notification sound" });
    await soundToggle.click();

    await expect(soundToggle).toHaveAttribute("data-settings-dirty", "true");
    await expect(testPage.getByTestId("notification-sound-group")).toHaveAttribute(
      "data-settings-dirty",
      "true",
    );
    expect(
      await testPage.evaluate(() =>
        JSON.parse(window.localStorage.getItem("kandev.notifications.sound") ?? "null"),
      ),
    ).toEqual({ enabled: false, presetId: "plim" });

    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .click();
    await expect(soundToggle).toHaveAttribute("data-settings-dirty", "false");
    expect(
      await testPage.evaluate(() =>
        JSON.parse(window.localStorage.getItem("kandev.notifications.sound") ?? "null"),
      ),
    ).toEqual({ enabled: true, presetId: "plim" });
  });
});
