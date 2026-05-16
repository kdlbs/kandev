import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

/**
 * CLI-mode parity: the create-task dialog must keep the prompt textarea
 * fully usable when the selected profile is a passthrough (CLI / PTY)
 * agent. The legacy "Prompt ignored — passthrough mode active" warning
 * has been removed; the backend now auto-injects the prompt into the
 * running CLI after the first idle window.
 */
test.describe("CLI mode: create-task dialog prompt", () => {
  test("prompt textarea is enabled, no 'prompt ignored' warning, and submit works", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Mark the pre-seeded agent profile as passthrough. The seedData
    // fixture pre-injects this profile id into localStorage, so the
    // create-task dialog auto-selects it without any UI plumbing.
    await apiClient.updateAgentProfile(seedData.agentProfileId, {
      cli_passthrough: true,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();

    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    // The prompt textarea is the one rendered inside the dialog body.
    const textarea = testPage.getByTestId("task-description-input");
    await expect(textarea).toBeVisible();
    await expect(textarea).toBeEnabled();

    // No legacy passthrough warning anywhere in the dialog.
    await expect(dialog).not.toContainText(/prompt ignored/i);
    await expect(dialog).not.toContainText(/passthrough mode active/i);
    await expect(dialog).not.toContainText(/prompt not supported/i);

    // The placeholder should not advertise "passthrough mode".
    const placeholder = (await textarea.getAttribute("placeholder")) ?? "";
    expect(placeholder.toLowerCase()).not.toContain("passthrough mode");

    // Fill title + description and confirm submit becomes enabled.
    await testPage.getByTestId("task-title-input").fill("CLI Mode Prompt Test");
    await textarea.fill("Refactor the cron handler");

    const startBtn = testPage.getByTestId("submit-start-agent");
    await expect(startBtn).toBeEnabled({ timeout: 30_000 });

    await startBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    // Navigated to the session page — confirms task created successfully.
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });
  });
});
