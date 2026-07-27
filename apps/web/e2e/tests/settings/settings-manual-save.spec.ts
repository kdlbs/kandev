import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

const APPEARANCE_PATH = "/settings/general/appearance";
const TERMINAL_PATH = "/settings/general/terminal";
const NOTIFICATIONS_PATH = "/settings/general/notifications";
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
      await expect(testPage.getByTestId("changes-panel-layout-card")).toHaveAttribute(
        "data-settings-dirty",
        "true",
      );
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
