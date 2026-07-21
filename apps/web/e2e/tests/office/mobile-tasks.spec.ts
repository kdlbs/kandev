import { test, expect } from "../../fixtures/office-fixture";

test.describe("Tasks on mobile", () => {
  test("shows subtasks expanded by default", async ({ testPage, apiClient, officeSeed }) => {
    const parentTitle = "Mobile Expanded Parent";
    const childTitle = "Mobile Expanded Child";
    const parent = await apiClient.createTask(officeSeed.workspaceId, parentTitle, {
      workflow_id: officeSeed.workflowId,
    });
    await apiClient.createTask(officeSeed.workspaceId, childTitle, {
      workflow_id: officeSeed.workflowId,
      parent_id: parent.id,
    });

    await testPage.goto("/office/tasks");

    await expect(testPage.getByText(parentTitle)).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText(childTitle)).toBeVisible();
  });
});
