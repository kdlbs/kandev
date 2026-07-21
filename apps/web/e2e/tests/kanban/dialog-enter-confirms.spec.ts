import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

const TASK_VISIBLE_TIMEOUT = 10_000;

// Pressing Enter on a dialog must execute its semantically focused action.
// For the delete-task confirm dialog that means the destructive "Delete"
// button, without the user having to click it or tab to it.
test.describe("Dialog Enter key — executes the semantic action", () => {
  test("pressing Enter on the delete-task confirm dialog deletes the task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Enter Delete Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await expect(kanban.taskCardByTitle("Enter Delete Task")).toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });

    await kanban.openTaskActionsMenu(task.id);
    await testPage.getByRole("menuitem", { name: "Delete" }).click();

    const dialog = testPage.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText("Enter Delete Task");

    // Do NOT click Delete — Enter alone must trigger the destructive action.
    await testPage.keyboard.press("Enter");

    await expect(dialog).not.toBeVisible();
    await expect(kanban.taskCardByTitle("Enter Delete Task")).not.toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });
  });
});
