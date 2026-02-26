import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";

test.describe("Task detail", () => {
  test("navigates to task detail from kanban card", async ({ testPage, apiClient, seedData }) => {
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

  test("task detail page loads for seeded task", async ({ testPage, apiClient, seedData }) => {
    await apiClient.createTask(seedData.workspaceId, "Direct Navigate Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    // The /tasks page shows all tasks for the workspace in a data table (SSR)
    await testPage.goto("/tasks");
    await testPage.waitForLoadState("networkidle");

    await expect(testPage.getByText("Direct Navigate Task")).toBeVisible();
  });
});
