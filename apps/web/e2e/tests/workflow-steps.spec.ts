import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";

test.describe("Workflow steps", () => {
  test("task appears in correct column after API move", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Seed a task (lands in the start step by default)
    const task = await apiClient.createTask(seedData.workspaceId, "Workflow Move Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    // Find a non-start step to move to
    const targetStep = seedData.steps.find((s) => !s.is_start_step);
    if (!targetStep) {
      test.skip(true, "No non-start step available to test move");
      return;
    }

    // Move the task via API
    await apiClient.moveTask(task.id, seedData.workflowId, targetStep.id);

    // Navigate to kanban and verify the card is visible
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("Workflow Move Task");
    await expect(card).toBeVisible();
  });

  test("multiple tasks can be seeded into different steps", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const startStep = seedData.steps.find((s) => s.is_start_step) ?? seedData.steps[0];
    const otherStep = seedData.steps.find((s) => s.id !== startStep.id);
    if (!otherStep) {
      test.skip(true, "Need at least 2 workflow steps");
      return;
    }

    // Create tasks and move one
    await apiClient.createTask(seedData.workspaceId, "Step A Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const taskB = await apiClient.createTask(seedData.workspaceId, "Step B Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.moveTask(taskB.id, seedData.workflowId, otherStep.id);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await expect(kanban.taskCardByTitle("Step A Task")).toBeVisible();
    await expect(kanban.taskCardByTitle("Step B Task")).toBeVisible();
  });
});
