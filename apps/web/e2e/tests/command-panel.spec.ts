import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";

/**
 * Helper: open the command panel via Cmd/Ctrl+K.
 */
async function openCommandPanel(page: import("@playwright/test").Page) {
  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await page.keyboard.press(`${modifier}+k`);
}

/**
 * Helper: open the file search panel via Cmd/Ctrl+Shift+K.
 */
async function openFileSearch(page: import("@playwright/test").Page) {
  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await page.keyboard.press(`${modifier}+Shift+k`);
}

/** The command dialog (Radix Dialog with role="dialog"). */
function commandDialog(page: import("@playwright/test").Page) {
  return page.getByRole("dialog");
}

test.describe("Command Panel", () => {
  test("Cmd+K opens command panel and shows commands (not files)", async ({
    testPage,
    seedData,
  }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await openCommandPanel(testPage);
    const dialog = commandDialog(testPage);
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    // Should show command groups like Navigation, Settings
    await expect(dialog.getByText("Navigation")).toBeVisible();
    await expect(dialog.getByText("Go to Home")).toBeVisible();

    // Type something that matches a navigation command
    await dialog.locator("input").fill("Settings");
    // Should find settings commands
    await expect(dialog.getByText("Go to Settings")).toBeVisible({ timeout: 5_000 });

    // Should NOT show a "Files" group — file search is now separate
    await expect(dialog.getByText("Files").first()).not.toBeVisible({ timeout: 2_000 });
  });

  test("Cmd+Shift+K opens file search mode with appropriate placeholder", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await openFileSearch(testPage);
    const dialog = commandDialog(testPage);
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    // Should show the "Files" back-button breadcrumb
    await expect(dialog.getByRole("button", { name: /Files/ })).toBeVisible();

    // Should show empty state for file search
    await expect(dialog.getByText("Type to search files...")).toBeVisible();
  });

  test("Cmd+K inline task search shows matching tasks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Create a task to search for
    await apiClient.createTask(seedData.workspaceId, "Searchable E2E Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // Wait for the task to appear on the kanban board first
    await expect(kanban.taskCardByTitle("Searchable E2E Task")).toBeVisible({ timeout: 10_000 });

    await openCommandPanel(testPage);
    const dialog = commandDialog(testPage);
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    // Type the task name — task search requires ≥2 characters
    await dialog.locator("input").fill("Searchable");

    // Should show the task in a "Tasks" group (inline search, debounced)
    await expect(dialog.getByText("Tasks")).toBeVisible({ timeout: 10_000 });
    await expect(dialog.getByText("Searchable E2E Task")).toBeVisible({ timeout: 5_000 });
  });

  test("inline task search shows workflow step badge", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.createTask(seedData.workspaceId, "Badged Task E2E", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCardByTitle("Badged Task E2E")).toBeVisible({ timeout: 10_000 });

    await openCommandPanel(testPage);
    const dialog = commandDialog(testPage);
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    await dialog.locator("input").fill("Badged Task");

    // The task should appear with a workflow step badge
    const taskRow = dialog.getByText("Badged Task E2E");
    await expect(taskRow).toBeVisible({ timeout: 10_000 });

    // The step badge should be present in the task result row
    const startStep = seedData.steps.find((s) => s.id === seedData.startStepId)!;
    const taskOption = dialog.getByRole("option", { name: /Badged Task E2E/ });
    await expect(taskOption).toBeVisible({ timeout: 5_000 });
    await expect(taskOption.getByText(startStep.name)).toBeVisible({ timeout: 5_000 });
  });

  test("Escape closes the command panel", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await openCommandPanel(testPage);
    const dialog = commandDialog(testPage);
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    await testPage.keyboard.press("Escape");
    await expect(dialog).not.toBeVisible({ timeout: 3_000 });
  });

  test("Backspace in file search mode returns to commands mode", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // Open file search mode
    await openFileSearch(testPage);
    const dialog = commandDialog(testPage);
    await expect(dialog).toBeVisible({ timeout: 5_000 });
    await expect(dialog.getByText("Type to search files...")).toBeVisible();

    // Focus the input and press backspace on empty input to go back to commands mode
    const input = dialog.getByRole("combobox");
    await input.focus();
    await input.press("Backspace");

    // Should now show the commands mode (navigation commands visible)
    await expect(dialog.getByText("Navigation")).toBeVisible({ timeout: 5_000 });
    await expect(dialog.getByText("Go to Home")).toBeVisible({ timeout: 3_000 });
  });

  test("Cmd+K toggles the panel open and closed", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // Open
    await openCommandPanel(testPage);
    const dialog = commandDialog(testPage);
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    // Close by pressing Cmd+K again
    await openCommandPanel(testPage);
    await expect(dialog).not.toBeVisible({ timeout: 3_000 });
  });
});
