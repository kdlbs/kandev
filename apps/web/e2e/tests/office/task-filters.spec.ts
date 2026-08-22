import { test, expect } from "../../fixtures/office-fixture";
import { officeTopbarTitle } from "../../helpers/office-topbar";

test.describe("Issue filters", () => {
  test("tasks page has toolbar", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office/tasks");
    await expect(officeTopbarTitle(testPage)).toHaveText(/Tasks/i, {
      timeout: 10_000,
    });
  });

  test("multiple tasks appear in tasks list", async ({ testPage, apiClient, officeSeed }) => {
    await apiClient.createTask(officeSeed.workspaceId, "Filter Task Alpha", {
      workflow_id: officeSeed.workflowId,
    });
    await apiClient.createTask(officeSeed.workspaceId, "Filter Task Beta", {
      workflow_id: officeSeed.workflowId,
    });

    await testPage.goto("/office/tasks");
    await expect(testPage.getByText("Filter Task Alpha")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText("Filter Task Beta")).toBeVisible({ timeout: 10_000 });
  });
});
