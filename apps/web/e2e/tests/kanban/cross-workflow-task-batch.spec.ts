import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

test.describe("Cross-workflow selected batch moves", () => {
  test("right-clicking a selected task sends the selected batch", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const targetWorkflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Batch Target Workflow",
    );
    const targetStep = await apiClient.createWorkflowStep(targetWorkflow.id, "Batch Incoming", 0);
    const [first, second, unselected] = await Promise.all([
      apiClient.createTask(seedData.workspaceId, "Batch Move First", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
      apiClient.createTask(seedData.workspaceId, "Batch Move Second", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
      apiClient.createTask(seedData.workspaceId, "Batch Move Unselected", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
    ]);
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.selectTask(first.id);
    await kanban.selectTask(second.id);
    await expect(kanban.multiSelectToolbar).toContainText("2 selected");

    await kanban.sendTaskToWorkflow(first.id, targetWorkflow.id, targetStep.id);

    await expect(kanban.taskCard(first.id)).not.toBeVisible({ timeout: 10_000 });
    await expect(kanban.taskCard(second.id)).not.toBeVisible();
    await expect(kanban.taskCard(unselected.id)).toBeVisible();

    await testPage.goto(`/?workflowId=${targetWorkflow.id}`);
    await expect(kanban.board).toBeVisible();
    await expect(kanban.taskCardInColumn("Batch Move First", targetStep.id)).toBeVisible({
      timeout: 10_000,
    });
    await expect(kanban.taskCardInColumn("Batch Move Second", targetStep.id)).toBeVisible();
    await expect(kanban.taskCardInColumn("Batch Move Unselected", targetStep.id)).not.toBeVisible();
  });

  test("right-clicking an unselected task moves only that task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const targetWorkflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Clicked Only Target Workflow",
    );
    const targetStep = await apiClient.createWorkflowStep(targetWorkflow.id, "Clicked Incoming", 0);
    const [selectedA, selectedB, clicked] = await Promise.all([
      apiClient.createTask(seedData.workspaceId, "Clicked Only Selected A", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
      apiClient.createTask(seedData.workspaceId, "Clicked Only Selected B", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
      apiClient.createTask(seedData.workspaceId, "Clicked Only Moved", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
    ]);
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.selectTask(selectedA.id);
    await kanban.selectTask(selectedB.id);
    await expect(kanban.multiSelectToolbar).toContainText("2 selected");

    await kanban.sendTaskToWorkflow(clicked.id, targetWorkflow.id, targetStep.id);

    await expect(kanban.taskCard(clicked.id)).not.toBeVisible({ timeout: 10_000 });
    await expect(kanban.taskCard(selectedA.id)).toBeVisible();
    await expect(kanban.taskCard(selectedB.id)).toBeVisible();
    await expect(kanban.multiSelectToolbar).toContainText("2 selected");

    await testPage.goto(`/?workflowId=${targetWorkflow.id}`);
    await expect(kanban.board).toBeVisible();
    await expect(kanban.taskCardInColumn("Clicked Only Moved", targetStep.id)).toBeVisible({
      timeout: 10_000,
    });
    await expect(
      kanban.taskCardInColumn("Clicked Only Selected A", targetStep.id),
    ).not.toBeVisible();
    await expect(
      kanban.taskCardInColumn("Clicked Only Selected B", targetStep.id),
    ).not.toBeVisible();
  });
});
