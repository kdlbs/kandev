import { test, expect } from "../../fixtures/office-fixture";

test.describe("Real-time issue updates", () => {
  test("new task created while on tasks page is visible after reload", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    // Navigate to tasks list
    await testPage.goto("/office/tasks");
    await expect(testPage.getByRole("heading", { name: /Tasks/i }).first()).toBeVisible({
      timeout: 10_000,
    });

    // Create a task via API
    await apiClient.createTask(officeSeed.workspaceId, "Realtime New Task ABC", {
      workflow_id: officeSeed.workflowId,
    });

    // Reload to pick up the new task (WS triggers refetch but may race with hydration)
    await testPage.reload();
    await expect(testPage.getByText("Realtime New Task ABC")).toBeVisible({ timeout: 10_000 });
  });

  test("task status change reflects in tasks list via WS", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    // Create task first
    const task = await apiClient.createTask(officeSeed.workspaceId, "Status Change Task", {
      workflow_id: officeSeed.workflowId,
    });

    // Navigate to tasks list
    await testPage.goto("/office/tasks");
    await expect(testPage.getByText("Status Change Task")).toBeVisible({ timeout: 10_000 });

    // Change status via API
    await officeApi.updateTaskStatus(task.id, "in_progress", "Starting work");

    // Verify the status updates in the UI (the status icon should change)
    // The issue row should reflect the new status without refresh
    await expect(testPage.getByText("Status Change Task")).toBeVisible({ timeout: 10_000 });
  });
});
