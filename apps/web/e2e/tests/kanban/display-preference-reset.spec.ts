import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

// The same worker persists user settings across these tests. The page fixture must
// restore column visibility before the next test uses the shared workflow.
test.describe.serial("Kanban display preference isolation", () => {
  test("persists hidden columns for the current test", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { steps } = await apiClient.listWorkflowSteps(seedData.workflowId);
    const review = steps.find((step) => step.name === "Review");
    expect(review).toBeDefined();
    const task = await apiClient.createTask(seedData.workspaceId, "Hidden review fixture", {
      workflow_id: seedData.workflowId,
      workflow_step_id: review!.id,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto(seedData.workflowId);
    await expect(kanban.taskCard(task.id)).toBeVisible();
    await apiClient.saveUserSettings({
      kanban_hidden_step_ids: { [seedData.workflowId]: [review!.id] },
      workflow_ids_with_auto_hide_empty_steps: [seedData.workflowId],
    });
    await testPage.reload();
    await expect(kanban.board).toBeVisible();
    await expect(kanban.columnByStepId(review!.id)).toHaveCount(0);
    await expect(kanban.taskCard(task.id)).toHaveCount(0);
  });

  test("restores visible columns for the next test", async ({ testPage, apiClient, seedData }) => {
    const { steps } = await apiClient.listWorkflowSteps(seedData.workflowId);
    const review = steps.find((step) => step.name === "Review");
    expect(review).toBeDefined();
    const task = await apiClient.createTask(seedData.workspaceId, "Visible review fixture", {
      workflow_id: seedData.workflowId,
      workflow_step_id: review!.id,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto(seedData.workflowId);
    await expect(kanban.columnByStepId(review!.id)).toBeVisible();
    await expect(kanban.taskCard(task.id)).toBeVisible();
  });
});
