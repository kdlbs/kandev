import { test, expect } from "../../fixtures/office-fixture";

test.describe("Issue sorting", () => {
  test("tasks list renders with default sort", async ({ testPage, apiClient, officeSeed }) => {
    await apiClient.createTask(officeSeed.workspaceId, "Sort Task One", {
      workflow_id: officeSeed.workflowId,
    });
    await apiClient.createTask(officeSeed.workspaceId, "Sort Task Two", {
      workflow_id: officeSeed.workflowId,
    });
    await testPage.goto("/office/tasks");
    // Both tasks should be visible in the sorted list
    await expect(testPage.getByText("Sort Task One")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText("Sort Task Two")).toBeVisible({ timeout: 10_000 });
  });

  test("issue rows show identifiers", async ({ testPage, apiClient, officeSeed }) => {
    await apiClient.createTask(officeSeed.workspaceId, "Identifier Sort Task", {
      workflow_id: officeSeed.workflowId,
    });
    await testPage.goto("/office/tasks");
    await expect(testPage.getByText("Identifier Sort Task")).toBeVisible({ timeout: 10_000 });
    // Verify identifier elements (font-mono class) are present in issue rows
    await expect(testPage.locator(".font-mono").first()).toBeVisible({ timeout: 5_000 });
  });
});
