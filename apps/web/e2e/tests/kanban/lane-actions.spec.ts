import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

test.describe("Lane actions", () => {
  test("hides lane menu when column has no tasks", async ({ testPage, seedData }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.columnByStepId(seedData.startStepId).hover();
    await expect(kanban.laneMenuTrigger(seedData.startStepId)).not.toBeVisible();
  });

  test("archive all removes tasks from lane", async ({ testPage, apiClient, seedData }) => {
    await apiClient.createTask(seedData.workspaceId, "Archive Task A", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.createTask(seedData.workspaceId, "Archive Task B", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await expect(kanban.taskCardByTitle("Archive Task A")).toBeVisible();
    await expect(kanban.taskCardByTitle("Archive Task B")).toBeVisible();

    await kanban.openLaneMenu(seedData.startStepId);
    await kanban.laneMenuArchiveAll().click();
    await kanban.laneConfirmArchive().click();

    await expect(kanban.taskCardByTitle("Archive Task A")).not.toBeVisible();
    await expect(kanban.taskCardByTitle("Archive Task B")).not.toBeVisible();
  });

  test("cancel archive keeps tasks in lane", async ({ testPage, apiClient, seedData }) => {
    await apiClient.createTask(seedData.workspaceId, "Keep Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.openLaneMenu(seedData.startStepId);
    await kanban.laneMenuArchiveAll().click();
    await testPage.getByRole("button", { name: "Cancel" }).click();

    await expect(kanban.taskCardByTitle("Keep Task")).toBeVisible();
  });

  test("clear lane deletes all tasks", async ({ testPage, apiClient, seedData }) => {
    await apiClient.createTask(seedData.workspaceId, "Delete Task A", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.createTask(seedData.workspaceId, "Delete Task B", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.openLaneMenu(seedData.startStepId);
    await kanban.laneMenuClear().click();
    await kanban.laneConfirmClear().click();

    await expect(kanban.taskCardByTitle("Delete Task A")).not.toBeVisible();
    await expect(kanban.taskCardByTitle("Delete Task B")).not.toBeVisible();
  });

  test("cancel clear keeps tasks in lane", async ({ testPage, apiClient, seedData }) => {
    await apiClient.createTask(seedData.workspaceId, "Survivor Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.openLaneMenu(seedData.startStepId);
    await kanban.laneMenuClear().click();
    await testPage.getByRole("button", { name: "Cancel" }).click();

    await expect(kanban.taskCardByTitle("Survivor Task")).toBeVisible();
  });

  test("move all to another lane", async ({ testPage, apiClient, seedData }) => {
    // Use Backlog (position 0) as the source — not the start step — so it's always a different step
    const backlogStep = seedData.steps.find((s) => s.position === 0 && !s.is_start_step);
    const targetStep = seedData.steps.find((s) => s.id !== backlogStep?.id);
    if (!backlogStep || !targetStep) throw new Error("Not enough workflow steps for move test");

    await apiClient.createTask(seedData.workspaceId, "Move Task A", {
      workflow_id: seedData.workflowId,
      workflow_step_id: backlogStep.id,
    });
    await apiClient.createTask(seedData.workspaceId, "Move Task B", {
      workflow_id: seedData.workflowId,
      workflow_step_id: backlogStep.id,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await expect(kanban.taskCardInColumn("Move Task A", backlogStep.id)).toBeVisible();
    await expect(kanban.taskCardInColumn("Move Task B", backlogStep.id)).toBeVisible();

    await kanban.openLaneMenu(backlogStep.id);
    await kanban.laneMenuMoveAll().hover();
    await kanban.laneMenuMoveToStep(targetStep.id).click();

    await expect(kanban.taskCardInColumn("Move Task A", targetStep.id)).toBeVisible();
    await expect(kanban.taskCardInColumn("Move Task B", targetStep.id)).toBeVisible();
    await expect(kanban.taskCardInColumn("Move Task A", backlogStep.id)).not.toBeVisible();
    await expect(kanban.taskCardInColumn("Move Task B", backlogStep.id)).not.toBeVisible();
  });
});
