import { test, expect } from "../../fixtures/test-base";

/**
 * Config chat popover UI tests.
 */

test.describe("Config chat popover", () => {
  test("opens popover, sends prompt, and shows agent response", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Set the mock agent profile as the default config chat profile.
    await apiClient.updateWorkspace(seedData.workspaceId, {
      default_config_agent_profile_id: seedData.agentProfileId,
    });

    // Navigate to settings page.
    await testPage.goto("/settings/agents");
    await testPage.waitForLoadState("networkidle");

    // Open the config chat popover.
    const fab = testPage.getByRole("button", { name: "Configuration Chat" });
    await expect(fab).toBeVisible({ timeout: 10_000 });
    await fab.click();

    const popover = testPage.locator("[data-radix-popper-content-wrapper]");
    await expect(popover).toBeVisible({ timeout: 5_000 });

    // Select profile if needed (appears when no default is set).
    const profileCard = popover.getByText("Mock Default", { exact: false });
    const textarea = popover.locator("textarea");
    if (await profileCard.isVisible()) {
      await profileCard.click();
      await expect(textarea).toBeVisible({ timeout: 5_000 });
    }

    // Submit prompt.
    await textarea.fill("Show me the current configuration");
    await textarea.press("Enter");

    // Wait for the tab to appear (means session was created).
    // Use the tab bar specifically to avoid matching message text.
    const tabBar = popover.locator(".scrollbar-hide");
    await expect(tabBar.getByText("Show me the cu", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // Wait for the agent response to appear in the chat area.
    const chatArea = popover.locator(".chat-message-list");
    await expect(chatArea.getByText("completed the analysis", { exact: false })).toBeVisible({
      timeout: 30_000,
    });
  });

  test("opens config chat via command panel (Cmd+K)", async ({ testPage, apiClient, seedData }) => {
    // Set the mock agent profile as the default config chat profile.
    await apiClient.updateWorkspace(seedData.workspaceId, {
      default_config_agent_profile_id: seedData.agentProfileId,
    });

    // Navigate to the kanban page (not settings) — FAB is hidden here,
    // but the command panel should still open config chat globally.
    await testPage.goto("/");
    await testPage.waitForLoadState("networkidle");

    // Open the command panel via Cmd+K.
    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await testPage.keyboard.press(`${modifier}+k`);

    const cmdPanel = testPage.getByRole("dialog", { name: "Command Palette" });
    await expect(cmdPanel).toBeVisible({ timeout: 5_000 });

    // Type to find the config chat command.
    await cmdPanel.locator("input").fill("Configuration Chat");
    await expect(cmdPanel.getByText("Configuration Chat")).toBeVisible({ timeout: 5_000 });

    // Select the command.
    await testPage.keyboard.press("Enter");

    // Command panel should close and config chat popover should open.
    await expect(cmdPanel).not.toBeVisible({ timeout: 3_000 });
    const popover = testPage.locator("[data-radix-popper-content-wrapper]");
    await expect(popover).toBeVisible({ timeout: 5_000 });
  });
});
