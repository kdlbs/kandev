import { test, expect } from "../../fixtures/test-base";

test.describe("Mobile task list search", () => {
  test("toolbar search filters the list", async ({ testPage, apiClient, seedData }) => {
    await apiClient.createTask(seedData.workspaceId, "List Alpha Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.createTask(seedData.workspaceId, "List Beta Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await testPage.goto("/tasks");
    await testPage.waitForLoadState("networkidle");

    const taskList = testPage.getByTestId("tasks-list");
    const searchInput = testPage.getByPlaceholder("Search tasks...");

    await expect(taskList.getByText("List Alpha Task")).toBeVisible();
    await expect(taskList.getByText("List Beta Task")).toBeVisible();

    await searchInput.fill("Alpha");
    await expect(taskList.getByText("List Alpha Task")).toBeVisible({ timeout: 5000 });
    await expect(taskList.getByText("List Beta Task")).not.toBeVisible({ timeout: 5000 });

    await searchInput.fill("");
    await expect(taskList.getByText("List Alpha Task")).toBeVisible({ timeout: 5000 });
    await expect(taskList.getByText("List Beta Task")).toBeVisible({ timeout: 5000 });
  });
});
