import { test, expect } from "../../fixtures/test-base";

test.describe("Keyboard Shortcuts Settings", () => {
  test.describe.configure({ retries: 1 });

  test("settings page shows all configurable shortcuts", async ({ testPage }) => {
    await testPage.goto("/settings/general");

    // Original 3 shortcuts
    await expect(testPage.getByTestId("shortcut-recorder-SEARCH")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByTestId("shortcut-recorder-FILE_SEARCH")).toBeVisible();
    await expect(testPage.getByTestId("shortcut-recorder-QUICK_CHAT")).toBeVisible();

    // Newly exposed shortcuts
    await expect(testPage.getByTestId("shortcut-recorder-BOTTOM_TERMINAL")).toBeVisible();
    await expect(testPage.getByTestId("shortcut-recorder-TOGGLE_SIDEBAR")).toBeVisible();
    await expect(testPage.getByTestId("shortcut-recorder-COMMAND_PANEL")).toBeVisible();
    await expect(testPage.getByTestId("shortcut-recorder-NEW_TASK")).toBeVisible();
    await expect(testPage.getByTestId("shortcut-recorder-FOCUS_INPUT")).toBeVisible();
    await expect(testPage.getByTestId("shortcut-recorder-TOGGLE_PLAN_MODE")).toBeVisible();
    await expect(testPage.getByTestId("shortcut-recorder-TASK_SWITCHER")).toBeVisible();
  });

  test("can record a new shortcut and persist it", async ({ testPage, apiClient, seedData }) => {
    // Reset settings to defaults
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      keyboard_shortcuts: {},
    });

    await testPage.goto("/settings/general");

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

    // Reload the page and verify the shortcut persisted
    await testPage.goto("/settings/general");
    const recorderAfterReload = testPage.getByTestId("shortcut-recorder-BOTTOM_TERMINAL");
    await expect(recorderAfterReload).toBeVisible({ timeout: 10_000 });
    await expect(recorderAfterReload).toContainText("T");
  });

  test("can reset a customized shortcut to default", async ({ testPage, apiClient, seedData }) => {
    // Set a custom shortcut via API
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      keyboard_shortcuts: {
        TOGGLE_SIDEBAR: { key: "x", modifiers: { ctrlOrCmd: true } },
      },
    });

    await testPage.goto("/settings/general");

    const recorder = testPage.getByTestId("shortcut-recorder-TOGGLE_SIDEBAR");
    await expect(recorder).toBeVisible({ timeout: 10_000 });

    // Should show the custom shortcut (X)
    await expect(recorder).toContainText("X", { timeout: 3_000 });

    // Should have a reset button since it's customized
    const row = recorder.locator("..");
    const resetButton = row.getByTitle("Reset to default");
    await expect(resetButton).toBeVisible();

    // Click reset
    await resetButton.click();

    // Should now show the default (B for Cmd/Ctrl+B)
    await expect(recorder).toContainText("B", { timeout: 3_000 });
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
