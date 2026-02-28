import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";

test.describe("Task List", () => {
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
