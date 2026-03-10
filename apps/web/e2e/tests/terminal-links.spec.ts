import { test, expect } from "../fixtures/test-base";

// ---------------------------------------------------------------------------
// Terminal clickable links — settings persistence and UI
// ---------------------------------------------------------------------------

test.describe("terminal clickable links", () => {
  test.beforeEach(async ({ apiClient }) => {
    await apiClient.saveUserSettings({ terminal_link_behavior: "new_tab" });
  });

  test("terminal_link_behavior defaults to new_tab and can be updated via API", async ({
    apiClient,
  }) => {
    // Default value
    const initial = await apiClient.getUserSettings();
    expect(initial.settings.terminal_link_behavior).toBe("new_tab");

    // Switch to browser_panel
    await apiClient.saveUserSettings({ terminal_link_behavior: "browser_panel" });
    const updated = await apiClient.getUserSettings();
    expect(updated.settings.terminal_link_behavior).toBe("browser_panel");

    // Revert to new_tab
    await apiClient.saveUserSettings({ terminal_link_behavior: "new_tab" });
    const reverted = await apiClient.getUserSettings();
    expect(reverted.settings.terminal_link_behavior).toBe("new_tab");
  });

  test("rejects invalid terminal_link_behavior values", async ({ apiClient }) => {
    const res = await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      terminal_link_behavior: "invalid_value",
    });
    expect(res.status).toBeGreaterThanOrEqual(400);
    expect(res.status).toBeLessThan(600);

    // Setting should remain unchanged
    const current = await apiClient.getUserSettings();
    expect(current.settings.terminal_link_behavior).toBe("new_tab");
  });

  test("settings page shows terminal links card and allows toggling", async ({ testPage }) => {
    await testPage.goto("/settings");

    // Terminal Links section visible
    await expect(testPage.locator("text=Terminal Links").first()).toBeVisible({ timeout: 10_000 });

    // Select trigger is visible
    const trigger = testPage.locator("#terminal-link-behavior");
    await expect(trigger).toBeVisible();

    // Switch to "Built-in browser panel"
    await trigger.click();
    const browserOption = testPage.getByRole("option", { name: "Built-in browser panel" });
    await expect(browserOption).toBeVisible();
    await browserOption.click();
    await expect(trigger).toHaveText(/Built-in browser panel/);

    // Switch back to "New browser tab"
    await trigger.click();
    const tabOption = testPage.getByRole("option", { name: "New browser tab" });
    await expect(tabOption).toBeVisible();
    await tabOption.click();
    await expect(trigger).toHaveText(/New browser tab/);
  });
});
