import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";

test.describe("Kanban board", () => {
  test("displays a seeded task card", async ({ testPage, apiClient, seedData }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "E2E Kanban Test Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);

    await kanban.goto();

    const card = kanban.taskCardByTitle("E2E Kanban Test Task");
    await expect(card).toBeVisible();
    await expect(kanban.taskCard(task.id)).toBeVisible();
  });

  test("shows create task button", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.createTaskButton.first()).toBeVisible();
  });
});
