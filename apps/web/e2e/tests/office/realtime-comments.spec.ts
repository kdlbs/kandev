import { test, expect } from "../../fixtures/office-fixture";

test.describe("Real-time comment updates", () => {
  test("new comment appears on issue detail via WS", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    // Create a task to view
    const task = await apiClient.createTask(officeSeed.workspaceId, "Comment WS Test", {
      workflow_id: officeSeed.workflowId,
    });

    // Navigate to the issue detail page
    await testPage.goto(`/office/tasks/${task.id}`);
    await expect(testPage.getByRole("heading", { name: "Comment WS Test" })).toBeVisible({
      timeout: 10_000,
    });

    // Create a comment via API while viewing the page
    await officeApi.createTaskComment(task.id, "This is a real-time comment");

    // The comment should appear without refresh
    await expect(testPage.getByText("This is a real-time comment")).toBeVisible({
      timeout: 15_000,
    });
  });
});
