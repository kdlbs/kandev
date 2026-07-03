import { test, expect } from "../../fixtures/test-base";

test.describe("Mobile task list search", () => {
  test("topbar search icon reveals, filters, and clears on collapse", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
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
    const searchBar = testPage.getByTestId("mobile-search-bar");
    const searchToggle = testPage.getByTestId("mobile-search-toggle");

    await expect(taskList.getByText("List Alpha Task")).toBeVisible();
    await expect(taskList.getByText("List Beta Task")).toBeVisible();
    await expect(searchBar).not.toBeVisible();

    await searchToggle.click();
    await expect(searchBar).toBeVisible();
    await expect(searchBar.getByPlaceholder("Search tasks...")).toBeFocused();

    await searchBar.getByPlaceholder("Search tasks...").fill("Alpha");
    await expect(taskList.getByText("List Alpha Task")).toBeVisible({ timeout: 5000 });
    await expect(taskList.getByText("List Beta Task")).not.toBeVisible({ timeout: 5000 });

    await searchToggle.click();
    await expect(searchBar).not.toBeVisible();
    await expect(taskList.getByText("List Alpha Task")).toBeVisible({ timeout: 5000 });
    await expect(taskList.getByText("List Beta Task")).toBeVisible({ timeout: 5000 });
  });
});
