import { test, expect } from "../../fixtures/office-fixture";

test.describe("Topbar breadcrumb", () => {
  test("issue detail shows task title", async ({ testPage, apiClient, officeSeed }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Breadcrumb Test Task", {
      workflow_id: officeSeed.workflowId,
    });
    await testPage.goto(`/office/tasks/${task.id}`);
    await expect(testPage.getByRole("heading", { name: "Breadcrumb Test Task" })).toBeVisible({
      timeout: 10_000,
    });
  });

  test("tasks list shows Tasks heading", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office/tasks");
    await expect(testPage.getByRole("heading", { name: /Tasks/i }).first()).toBeVisible({
      timeout: 10_000,
    });
  });
});
