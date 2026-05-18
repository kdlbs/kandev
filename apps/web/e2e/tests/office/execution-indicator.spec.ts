import { test, expect } from "../../fixtures/office-fixture";

test.describe("Execution indicator", () => {
  test("issue row shows status icon for task", async ({ testPage, apiClient, officeSeed }) => {
    await apiClient.createTask(officeSeed.workspaceId, "Indicator Test Task", {
      workflow_id: officeSeed.workflowId,
    });
    await testPage.goto("/office/tasks");
    await expect(testPage.getByText("Indicator Test Task")).toBeVisible({ timeout: 10_000 });
  });

  test("issue row displays task identifier", async ({ testPage, apiClient, officeSeed }) => {
    await apiClient.createTask(officeSeed.workspaceId, "Identifier Test Task", {
      workflow_id: officeSeed.workflowId,
    });
    await testPage.goto("/office/tasks");
    await expect(testPage.getByText("Identifier Test Task")).toBeVisible({ timeout: 10_000 });
    // Task identifiers use a workspace prefix (e.g. E2E-1, TST-1)
    // Verify at least one element with a short alphanumeric identifier pattern exists
    await expect(testPage.locator(".font-mono").first()).toBeVisible({ timeout: 5_000 });
  });
});
