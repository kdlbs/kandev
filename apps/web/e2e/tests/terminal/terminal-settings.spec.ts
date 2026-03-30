import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

// The full font stack value for "JetBrains Mono" preset
const JB_MONO_STACK = '"JetBrains Mono", "Fira Code", Menlo, Consolas, monospace';

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

/** Read the xterm font family from the terminal panel via the exposed helper. */
async function readTerminalFontFamily(page: Page): Promise<string> {
  return page.evaluate(() => {
    const panel = document.querySelector('[data-testid="terminal-panel"]');
    if (!panel) return "";
    const xtermEl = panel.querySelector(".xterm");
    type XC = HTMLElement & { __xtermGetFontFamily?: () => string };
    const container = xtermEl?.parentElement as XC | null | undefined;
    return container?.__xtermGetFontFamily?.() ?? "";
  });
}

/** Read the xterm font size from the terminal panel via the exposed helper. */
async function readTerminalFontSize(page: Page): Promise<number> {
  return page.evaluate(() => {
    const panel = document.querySelector('[data-testid="terminal-panel"]');
    if (!panel) return 0;
    const xtermEl = panel.querySelector(".xterm");
    type XC = HTMLElement & { __xtermGetFontSize?: () => number };
    const container = xtermEl?.parentElement as XC | null | undefined;
    return container?.__xtermGetFontSize?.() ?? 0;
  });
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe("Terminal font settings", () => {
  test.describe.configure({ retries: 1 });

  test("font family persists after page reload", async ({ testPage, apiClient, seedData }) => {
    // Seed font via API, then verify through UI
    await apiClient.saveUserSettings({ terminal_font_family: JB_MONO_STACK });

    const session = await seedTaskWithSession(testPage, apiClient, seedData, "Font Persist");
    await expect(session.terminal).toBeVisible({ timeout: 15_000 });

    // Verify font is applied
    const fontBefore = await readTerminalFontFamily(testPage);
    expect(fontBefore).toContain("JetBrains Mono");

    // Reload and verify persistence
    await testPage.reload();
    await session.waitForLoad();
    await expect(session.terminal).toBeVisible({ timeout: 15_000 });

    const fontAfter = await readTerminalFontFamily(testPage);
    expect(fontAfter).toContain("JetBrains Mono");

    // Clean up
    await apiClient.saveUserSettings({ terminal_font_family: "" });
  });

  test("font size persists after page reload", async ({ testPage, apiClient, seedData }) => {
    await apiClient.saveUserSettings({ terminal_font_size: 18 });

    const session = await seedTaskWithSession(testPage, apiClient, seedData, "Size Persist");
    await expect(session.terminal).toBeVisible({ timeout: 15_000 });

    const sizeBefore = await readTerminalFontSize(testPage);
    expect(sizeBefore).toBe(18);

    // Reload and verify persistence
    await testPage.reload();
    await session.waitForLoad();
    await expect(session.terminal).toBeVisible({ timeout: 15_000 });

    const sizeAfter = await readTerminalFontSize(testPage);
    expect(sizeAfter).toBe(18);

    // Clean up
    await apiClient.saveUserSettings({ terminal_font_size: 0 });
  });

  test("settings page shows font and size controls", async ({ testPage }) => {
    await testPage.goto("/settings/general");

    // Font family selector
    const fontSelect = testPage.getByTestId("terminal-font-select");
    await expect(fontSelect).toBeVisible({ timeout: 10_000 });
    await fontSelect.click();

    // Verify a preset is listed (exact match to avoid matching Nerd Font variant)
    const option = testPage.getByRole("option", { name: "JetBrains Mono", exact: true });
    await expect(option).toBeVisible({ timeout: 5_000 });

    // Close dropdown by pressing Escape
    await testPage.keyboard.press("Escape");

    // Font size input
    const fontSizeInput = testPage.getByTestId("terminal-font-size-input");
    await expect(fontSizeInput).toBeVisible();
  });
});

test.describe("Terminal link settings", () => {
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
    expect(res.status).toBeLessThan(500);

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
