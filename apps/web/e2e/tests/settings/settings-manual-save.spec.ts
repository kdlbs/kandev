import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

const APPEARANCE_PATH = "/settings/general/appearance";
const TERMINAL_PATH = "/settings/general/terminal";
const NOTIFICATIONS_PATH = "/settings/general/notifications";
const TASK_ACTIONS_PATH = "/settings/general/task-actions";
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
  test("persists host sleep inhibition only when Save changes is pressed", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    const initialResponse = await apiClient.rawRequest("GET", "/api/v1/system/sleep-inhibition");
    expect(initialResponse.ok).toBe(true);
    const initial = (await initialResponse.json()) as {
      settings: { enabled: boolean };
    };
    const next = !initial.settings.enabled;

    try {
      await testPage.goto(TASK_ACTIONS_PATH);
      const card = testPage.getByTestId("sleep-inhibition-settings");
      const toggle = card.getByRole("switch", { name: "Prevent idle system sleep" });
      await expect(card).toBeVisible();
      await expect(card).toContainText("Container, Kubernetes, remote-executor");
      const info = card.getByRole("button", { name: "How host sleep prevention works" });
      await info.hover();
      const infoTooltip = testPage.getByRole("tooltip");
      await expect(infoTooltip).toContainText("/usr/bin/caffeinate -i -w");
      await expect(infoTooltip).toContainText("SetThreadExecutionState");
      await expect(infoTooltip).toContainText("systemd-logind");
      await testPage.waitForTimeout(500);
      await prCapture.screenshot("sleep-inhibition-desktop-info", {
        caption: "Desktop host sleep prevention details in the hover tooltip",
      });
      if (initial.settings.enabled) await expect(toggle).toBeChecked();
      else await expect(toggle).not.toBeChecked();

      await toggle.click();
      await expect(card).toHaveAttribute("data-settings-dirty", "true");
      const beforeSaveResponse = await apiClient.rawRequest(
        "GET",
        "/api/v1/system/sleep-inhibition",
      );
      expect(((await beforeSaveResponse.json()) as typeof initial).settings.enabled).toBe(
        initial.settings.enabled,
      );

      const floatingSave = testPage.getByTestId("settings-floating-save");
      await testPage.waitForTimeout(1_000);
      await testPage
        .locator("[data-sonner-toast], [data-testid='toast-message']")
        .evaluateAll((toasts) => {
          for (const toast of toasts) (toast as HTMLElement).style.display = "none";
        });
      await prCapture.screenshot("sleep-inhibition-desktop-draft", {
        caption: "Task Actions sleep inhibition setting with Save changes pending",
      });
      await floatingSave.getByRole("button", { name: "Save changes" }).click();
      await expect(floatingSave).not.toBeVisible({ timeout: 15_000 });

      const savedResponse = await apiClient.rawRequest("GET", "/api/v1/system/sleep-inhibition");
      expect(((await savedResponse.json()) as typeof initial).settings.enabled).toBe(next);
      await testPage.reload();
      const reloadedToggle = testPage
        .getByTestId("sleep-inhibition-settings")
        .getByRole("switch", { name: "Prevent idle system sleep" });
      if (next) await expect(reloadedToggle).toBeChecked();
      else await expect(reloadedToggle).not.toBeChecked();
    } finally {
      await apiClient.rawRequest("PATCH", "/api/v1/system/sleep-inhibition", {
        enabled: initial.settings.enabled,
      });
    }
  });

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

  test("persists transcript navigation choices only when Save changes is pressed", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    const initial = await apiClient.getUserSettings();
    const initialAutoScrollControl = initial.settings.show_transcript_auto_scroll_control;

    try {
      await testPage.goto("/settings/general/task-actions");
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
