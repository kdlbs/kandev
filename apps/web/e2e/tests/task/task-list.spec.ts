import { test, expect } from "../../fixtures/test-base";

test.describe("Task List", () => {
  test("seeded task appears in task list", async ({ testPage, apiClient, seedData }) => {
    await apiClient.createTask(seedData.workspaceId, "Direct Navigate Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    // The /tasks page shows all tasks for the workspace in the list view (SSR)
    await testPage.goto("/tasks");
    await testPage.waitForLoadState("networkidle");

    // The global AppSidebar also lists tasks (data-testid="sidebar-task-item"),
    // so a bare getByText matches two elements. Scope to the page list.
    await expect(
      testPage.getByTestId("tasks-list").getByText("Direct Navigate Task"),
    ).toBeVisible();
    await expect(testPage.getByTestId("kanban-header-search")).toBeVisible();
    await expect(testPage.locator("main").getByRole("button", { name: "New Task" })).toHaveCount(0);
  });

  test("subtasks render under their parent with indentation", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const parent = await apiClient.createTask(seedData.workspaceId, "Hierarchy Parent Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.createTask(seedData.workspaceId, "Hierarchy Child Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      parent_id: parent.id,
    });

    await testPage.goto("/tasks");
    await testPage.waitForLoadState("networkidle");
    await testPage
      .getByTestId("kanban-header-search")
      .getByPlaceholder("Search tasks...")
      .fill("Hierarchy");

    const taskList = testPage.getByTestId("tasks-list");
    const parentRow = taskList.getByTestId("tasks-list-row").filter({
      hasText: "Hierarchy Parent Task",
    });
    const childRow = taskList.getByTestId("tasks-list-row").filter({
      hasText: "Hierarchy Child Task",
    });

    await expect(parentRow).toHaveAttribute("data-level", "0", { timeout: 5000 });
    await expect(childRow).toHaveAttribute("data-level", "1", { timeout: 5000 });
    await expect(parentRow).toHaveCount(1);
    await expect(childRow).toHaveCount(1);
  });
});
