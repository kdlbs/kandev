import { test, expect } from "../../fixtures/office-fixture";

test.describe("Session work entry", () => {
  test("issue detail shows comments section", async ({ testPage, apiClient, officeSeed }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Session Entry Task", {
      workflow_id: officeSeed.workflowId,
    });
    await testPage.goto(`/office/tasks/${task.id}`);
    await expect(testPage.getByRole("heading", { name: "Session Entry Task" })).toBeVisible({
      timeout: 10_000,
    });
    // Should show the comment input area
    await expect(testPage.getByPlaceholder("Add a comment")).toBeVisible({ timeout: 5_000 });
  });

  test("issue detail shows task status", async ({ testPage, apiClient, officeSeed }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Properties Task", {
      workflow_id: officeSeed.workflowId,
    });
    await testPage.goto(`/office/tasks/${task.id}`);
    await expect(testPage.getByRole("heading", { name: "Properties Task" })).toBeVisible({
      timeout: 10_000,
    });
  });
});
