import { type Page } from "@playwright/test";
import { test, expect } from "../fixtures/test-base";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "../helpers/api-client";
import { SessionPage } from "../pages/session-page";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Create a non-TUI task and navigate to its session. Waits for agent idle. */
async function seedTaskWithSession(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
  return session;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe("Terminal font settings", () => {
  // Standalone executor can fail on cold start; retry once for transient failures.
  test.describe.configure({ retries: 1 });

  test("font setting persists via API", async ({ apiClient }) => {
    await apiClient.saveUserSettings({ terminal_font_family: "JetBrains Mono" });

    const { settings } = await apiClient.getUserSettings();
    expect(settings.terminal_font_family).toBe("JetBrains Mono");

    // Clean up
    await apiClient.saveUserSettings({ terminal_font_family: "" });
  });

  test("terminal uses custom font after page load", async ({ testPage, apiClient, seedData }) => {
    await apiClient.saveUserSettings({ terminal_font_family: "JetBrains Mono" });

    const session = await seedTaskWithSession(testPage, apiClient, seedData, "Font Check");

    await expect(session.terminal).toBeVisible({ timeout: 15_000 });

    // xterm.js applies fontFamily via canvas rendering, not CSS.
    // The .xterm-helper-textarea gets the font for measuring, so read from there.
    const fontFamily = await testPage.evaluate(() => {
      const panel = document.querySelector('[data-testid="terminal-panel"]');
      if (!panel) return "";
      const textarea = panel.querySelector<HTMLTextAreaElement>(".xterm-helper-textarea");
      if (textarea)
        return textarea.style.fontFamily || window.getComputedStyle(textarea).fontFamily;
      return "";
    });

    expect(fontFamily).toContain("JetBrains Mono");

    // Clean up
    await apiClient.saveUserSettings({ terminal_font_family: "" });
  });

  test("settings page shows font selector", async ({ testPage }) => {
    await testPage.goto("/settings/general");

    const fontSelect = testPage.getByTestId("terminal-font-select");
    await expect(fontSelect).toBeVisible({ timeout: 10_000 });

    // Open the dropdown
    await fontSelect.click();

    // Verify JetBrains Mono is listed as an option (exact match to avoid
    // matching "JetBrains Mono Nerd Font" as well)
    const option = testPage.getByRole("option", { name: "JetBrains Mono", exact: true });
    await expect(option).toBeVisible({ timeout: 5_000 });
  });
});
