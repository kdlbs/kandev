import { expect, test } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";

useRegularMode();

test.describe("Task create relationships", () => {
  test("selects a parent and related task from advanced settings", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const parent = await apiClient.createTask(seedData.workspaceId, "Relationship parent", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const related = await apiClient.createTask(
      seedData.workspaceId,
      "Relationship related candidate",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      },
    );
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await dialog.getByTestId("task-create-advanced-settings-trigger").click();

    const parentTrigger = dialog.getByTestId("task-create-parent-trigger");
    await parentTrigger.click();
    await dialog.getByTestId(`task-create-parent-option-${parent.id}`).click();
    const relatedTrigger = dialog.getByTestId("task-create-related-trigger");
    await relatedTrigger.click();
    await expect(dialog.getByTestId(`task-create-related-option-${parent.id}`)).toHaveCount(0);
    await dialog.getByTestId(`task-create-related-option-${related.id}`).click();
    await expect(parentTrigger).toContainText("Relationship parent");
    await expect(relatedTrigger).toContainText("1 related task");
  });
});
