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

  test("opens task preview from kanban card", async ({ testPage, apiClient, seedData }) => {
    // Enable preview-on-click so clicking a card opens the preview panel
    await apiClient.saveUserSettings({ enable_preview_on_click: true });

    const task = await apiClient.createTask(seedData.workspaceId, "Detail View Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);

    await kanban.goto();

    const card = kanban.taskCard(task.id);
    await expect(card).toBeVisible();
    await card.click();

    // After clicking a card with preview enabled, URL gets ?taskId=... param.
    // Use toHaveURL (polling assertion) since replaceState doesn't fire navigation events.
    await expect(testPage).toHaveURL(/taskId=/, { timeout: 10000 });
  });
});
