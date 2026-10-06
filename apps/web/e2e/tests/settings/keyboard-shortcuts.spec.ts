import {
  INTEGRATION_CHORD,
  recordIntegrationShortcut,
  saveIntegrationShortcuts,
} from "./integration-shortcuts-helpers";
import {
  openInstallDialog,
  uploadPackage,
  uninstallPluginFixture,
  PACKAGE_PATH,
  PLUGIN_ID,
} from "../plugins/plugin-test-helpers";
import { test, expect } from "../../fixtures/test-base";

const KEYBOARD_SETTINGS_PATH = "/settings/preferences/keyboard-shortcuts";

test.describe("Keyboard Shortcuts Settings", () => {
  test.describe.configure({ retries: 1 });

  test("settings page shows all configurable shortcuts", async ({ testPage }) => {
    await testPage.goto(KEYBOARD_SETTINGS_PATH);

    await expect(testPage.locator("#chat-submit-key")).toBeVisible({ timeout: 10_000 });

    // Original 3 shortcuts
    await expect(testPage.getByTestId("shortcut-recorder-SEARCH")).toBeVisible();
    await expect(testPage.getByTestId("shortcut-recorder-FILE_SEARCH")).toBeVisible();
    await expect(testPage.getByTestId("shortcut-recorder-QUICK_CHAT")).toBeVisible();

    // Newly exposed shortcuts
    await expect(testPage.getByTestId("shortcut-recorder-BOTTOM_TERMINAL")).toBeVisible();
    await expect(testPage.getByTestId("shortcut-recorder-TOGGLE_SIDEBAR")).toBeVisible();
    await expect(testPage.getByTestId("shortcut-recorder-COMMAND_PANEL")).toBeVisible();
    await expect(testPage.getByTestId("shortcut-recorder-NEW_TASK")).toBeVisible();
    await expect(testPage.getByTestId("shortcut-recorder-FOCUS_INPUT")).toBeVisible();
    await expect(testPage.getByTestId("shortcut-recorder-FOCUS_PASSTHROUGH_INPUT")).toBeVisible();
    await expect(testPage.getByText("Focus CLI Chat Input")).toBeVisible();
    await expect(testPage.getByTestId("shortcut-recorder-TOGGLE_PLAN_MODE")).toBeVisible();
    await expect(testPage.getByTestId("shortcut-recorder-TASK_SWITCHER")).toBeVisible();
    await expect(testPage.getByTestId("shortcut-recorder-TASK_SWITCHER_REVERSE")).toBeVisible();
    await expect(testPage.getByTestId("shortcut-recorder-REVERSE_SEARCH")).toBeVisible();
    await expect(testPage.getByTestId("shortcut-recorder-WORKSPACE_PICKER")).toBeVisible();
    await expect(testPage.getByText("Open Workspace Picker")).toBeVisible();
  });

  test("can record a new shortcut and persist it", async ({ testPage, apiClient, seedData }) => {
    // Reset settings to defaults
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      keyboard_shortcuts: {},
    });

    await testPage.goto(KEYBOARD_SETTINGS_PATH);

    // Find the BOTTOM_TERMINAL recorder and verify it shows the default (Cmd+J or Ctrl+J)
    const recorder = testPage.getByTestId("shortcut-recorder-BOTTOM_TERMINAL");
    await expect(recorder).toBeVisible({ timeout: 10_000 });

    // Click to start recording
    await recorder.click();
    await expect(recorder.getByText("Press a key combo...")).toBeVisible({ timeout: 3_000 });

    // Press a new key combo: Ctrl+Shift+T
    await testPage.keyboard.press("Control+Shift+t");

    // Verify the recorder displays the new combo (recording should have stopped)
    await expect(recorder.getByText("Press a key combo...")).not.toBeVisible({ timeout: 3_000 });
    // The Kbd element should now show the new shortcut
    await expect(recorder).toContainText("T", { timeout: 3_000 });

    const floatingSave = testPage.getByTestId("settings-floating-save");
    await floatingSave.getByRole("button", { name: "Save changes" }).click();
    await expect(floatingSave).not.toBeVisible({ timeout: 10_000 });

    // Reload the page and verify the shortcut persisted
    await testPage.goto(KEYBOARD_SETTINGS_PATH);
    const recorderAfterReload = testPage.getByTestId("shortcut-recorder-BOTTOM_TERMINAL");
    await expect(recorderAfterReload).toBeVisible({ timeout: 10_000 });
    await expect(recorderAfterReload).toContainText("T");
  });

  test("can reset a customized shortcut back to unbound", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Set a custom shortcut via API
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      keyboard_shortcuts: {
        TOGGLE_SIDEBAR: { key: "x", modifiers: { ctrlOrCmd: true } },
      },
    });

    await testPage.goto(KEYBOARD_SETTINGS_PATH);

    const recorder = testPage.getByTestId("shortcut-recorder-TOGGLE_SIDEBAR");
    await expect(recorder).toBeVisible({ timeout: 10_000 });

    // Should show the custom shortcut (X)
    await expect(recorder).toContainText("X", { timeout: 3_000 });

    // Click reset (TOGGLE_SIDEBAR has no default binding, so reset clears it)
    const row = recorder.locator("..");
    const resetButton = row.getByTitle(/Reset/);
    await expect(resetButton).toBeVisible();
    await resetButton.click();

    // Should now show "Unbound"
    await expect(recorder).toContainText("Unbound", { timeout: 3_000 });
  });

  test("customized command panel shortcut opens the panel", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Override SEARCH (command panel) shortcut to Ctrl+Shift+O
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      keyboard_shortcuts: {
        SEARCH: { key: "o", modifiers: { ctrlOrCmd: true, shift: true } },
      },
    });

    await testPage.goto("/");
    await testPage.waitForLoadState("networkidle");

    // Press the NEW shortcut — command panel should open
    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await testPage.keyboard.press(`${modifier}+Shift+o`);

    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    // Close the dialog
    await testPage.keyboard.press("Escape");
    await expect(dialog).not.toBeVisible({ timeout: 3_000 });
  });

  test("customized task switcher shortcut opens the recent task switcher", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      keyboard_shortcuts: {
        TASK_SWITCHER: { key: "y", modifiers: { ctrlOrCmd: true, shift: true } },
      },
    });

    await testPage.goto("/");
    await testPage.waitForLoadState("networkidle");

    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await testPage.keyboard.down(modifier);
    await testPage.keyboard.down("Shift");
    await testPage.keyboard.press("y");

    await expect(testPage.getByTestId("recent-task-switcher")).toBeVisible({ timeout: 5_000 });
    await testPage.keyboard.up(modifier);
    await testPage.keyboard.up("Shift");
    await expect(testPage.getByTestId("recent-task-switcher")).not.toBeVisible({ timeout: 5_000 });
  });
});

// @covers AC-UI-INTEGRATION-PAGE-SHORTCUTS-001.1-.6, .8
// Unit tests provide behavioral RED; these scenarios prove real routing and persistence.
test.describe("Integration navigation hotkeys", () => {
  let baseline: Record<string, unknown>;
  test.beforeEach(async ({ testPage, apiClient }) => {
    void testPage;
    baseline =
      ((await apiClient.getUserSettings()).settings.keyboard_shortcuts as Record<
        string,
        unknown
      >) ?? {};
    await apiClient.saveUserSettings({ keyboard_shortcuts: {} });
  });
  test.afterEach(async ({ apiClient }) => {
    await apiClient.saveUserSettings({ keyboard_shortcuts: baseline });
  });

  test("keeps a draft through phone and desktop boundaries with usable controls", async ({
    testPage,
  }, testInfo) => {
    await testPage.goto(KEYBOARD_SETTINGS_PATH);
    const recorder = await recordIntegrationShortcut(testPage, "github");
    const row = recorder.locator("..").locator("..");
    const label = row.getByText("Open GitHub", { exact: true });
    for (const width of [767, 768, 1280]) {
      await testPage.setViewportSize({ width, height: 900 });
      await expect(recorder).toContainText("G");
      await expect(recorder).toHaveAttribute("data-settings-dirty", "true");
      await expect(row).toHaveCSS("flex-direction", width < 768 ? "column" : "row");
      await recorder.scrollIntoViewIfNeeded();
      const [box, labelBox] = await Promise.all([recorder.boundingBox(), label.boundingBox()]);
      if (width < 768) {
        expect(box!.height).toBeGreaterThanOrEqual(44);
        expect(box!.y).toBeGreaterThanOrEqual(labelBox!.y + labelBox!.height);
      } else {
        expect(box!.height).toBeGreaterThanOrEqual(28);
        expect(box!.height).toBeLessThan(44);
      }
      expect(
        await testPage.evaluate(
          () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
        ),
      ).toBe(false);
    }
    await testPage.screenshot({ path: testInfo.outputPath("integration-shortcuts-desktop.png") });
    await saveIntegrationShortcuts(testPage);
    await testPage.reload();
    await expect(recorder).toContainText("G");
  });

  test("records, saves, reloads, opens and resets every integration page", async ({
    testPage,
    apiClient,
  }) => {
    const destinations = [
      ["azure-devops", "/azure-devops"],
      ["github", "/github"],
      ["gitlab", "/gitlab"],
      ["jira", "/jira"],
      ["linear", "/linear"],
      ["sentry", "/settings/integrations/sentry"],
    ];
    await testPage.goto(KEYBOARD_SETTINGS_PATH);
    for (const [slug] of destinations) {
      await expect(testPage.getByTestId(`shortcut-recorder-integration:${slug}`)).toContainText(
        "Unbound",
      );
    }
    for (const [slug, destination] of destinations) {
      await recordIntegrationShortcut(testPage, slug);
      await saveIntegrationShortcuts(testPage);
      await testPage.reload();
      const recorder = testPage.getByTestId(`shortcut-recorder-integration:${slug}`);
      await expect(recorder).toContainText("G");
      // Re-recording the already saved chord must stay in settings.
      await recordIntegrationShortcut(testPage, slug);
      await expect(testPage).toHaveURL(new RegExp(`${KEYBOARD_SETTINGS_PATH}$`));
      await testPage.goto("/");
      await testPage.keyboard.press(INTEGRATION_CHORD);
      await expect(testPage).toHaveURL(new RegExp(`${destination}$`));
      await testPage.goto(KEYBOARD_SETTINGS_PATH);
      await recorder.locator("..").getByRole("button", { name: "Reset (clear shortcut)" }).click();
      await saveIntegrationShortcuts(testPage);
      await testPage.reload();
      await expect(recorder).toContainText("Unbound");
      expect(
        (await apiClient.getUserSettings()).settings.keyboard_shortcuts ?? {},
      ).not.toHaveProperty(`integration:${slug}`);
    }
    await testPage.goto("/");
    await testPage.keyboard.press("Control+k");
    await expect(testPage.getByRole("dialog")).toBeVisible();
  });

  test("warns about conflicting navigation chords and navigates once", async ({ testPage }) => {
    await testPage.goto(KEYBOARD_SETTINGS_PATH);
    await recordIntegrationShortcut(testPage, "github");
    await recordIntegrationShortcut(testPage, "jira");
    await expect(testPage.getByTitle("Same shortcut as: Open Jira")).toBeVisible();
    await expect(testPage.getByTitle("Same shortcut as: Open GitHub")).toBeVisible();
    await saveIntegrationShortcuts(testPage);
    await testPage.goto("/");
    await testPage.keyboard.press(INTEGRATION_CHORD);
    await expect(testPage).toHaveURL(/\/github$/);
  });

  test("keeps failed saves retryable without activating the draft", async ({ testPage }) => {
    await testPage.goto(KEYBOARD_SETTINGS_PATH);
    const recorder = await recordIntegrationShortcut(testPage, "github");
    await testPage.route("**/api/v1/user/settings", async (route) => {
      if (route.request().method() === "PATCH")
        await route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({ error: "Save failed" }),
        });
      else await route.continue();
    });
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .click();
    await expect(testPage.getByRole("button", { name: "Retry save" })).toBeVisible();
    await expect(recorder).toHaveAttribute("data-settings-dirty", "true");
    await testPage.unroute("**/api/v1/user/settings");
    await testPage.getByRole("button", { name: "Retry save" }).click();
    await expect(testPage.getByTestId("settings-floating-save")).not.toBeVisible();
    await testPage.reload();
    await expect(recorder).toContainText("G");
  });

  test("opens a packaged plugin integration and retains its binding across disable", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(120_000);
    try {
      await openInstallDialog(testPage);
      await uploadPackage(testPage, PACKAGE_PATH);
      await expect(testPage.getByTestId(`plugin-row-${PLUGIN_ID}`)).toContainText("Active", {
        timeout: 30_000,
      });
      await testPage.goto(KEYBOARD_SETTINGS_PATH);
      const slug = `plugin:${PLUGIN_ID}:e2e-integration`;
      const recorder = await recordIntegrationShortcut(testPage, slug);
      await saveIntegrationShortcuts(testPage);
      await testPage.goto("/");
      await expect(testPage.getByTestId("plugin-nav-item-e2e-hello")).toBeVisible();
      await testPage.keyboard.press(INTEGRATION_CHORD);
      await expect(testPage.locator("#hello-plugin-page")).toBeVisible();
      await testPage.goto(`/settings/plugins/${PLUGIN_ID}`);
      await testPage.getByRole("button", { name: "Disable", exact: true }).click();
      await expect(testPage.getByRole("button", { name: "Enable", exact: true })).toBeVisible();
      await testPage.goto(KEYBOARD_SETTINGS_PATH);
      await expect(recorder).toHaveCount(0);
      expect((await apiClient.getUserSettings()).settings.keyboard_shortcuts).toHaveProperty(
        `integration:${slug}`,
      );
      await testPage.goto(`/settings/plugins/${PLUGIN_ID}`);
      await testPage.getByRole("button", { name: "Enable", exact: true }).click();
      await expect(testPage.getByRole("button", { name: "Disable", exact: true })).toBeVisible();
      await testPage.goto("/");
      await expect(testPage.getByTestId("plugin-nav-item-e2e-hello")).toBeVisible();
      await testPage.keyboard.press(INTEGRATION_CHORD);
      await expect(testPage.locator("#hello-plugin-page")).toBeVisible();
    } finally {
      await uninstallPluginFixture(apiClient);
    }
  });
});
