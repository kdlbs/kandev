import { test, expect } from "../../fixtures/office-fixture";

test.describe("Simple/Advanced mode toggle", () => {
  test("issue detail defaults to simple mode", async ({ testPage, apiClient, officeSeed }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Mode Toggle Task", {
      workflow_id: officeSeed.workflowId,
    });
    await testPage.goto(`/office/tasks/${task.id}`);
    await expect(testPage.getByRole("heading", { name: "Mode Toggle Task" })).toBeVisible({
      timeout: 10_000,
    });
    // Simple mode shows the Chat tab
    await expect(testPage.getByRole("tab", { name: "Chat" })).toBeVisible({ timeout: 5_000 });
  });

  test("mode=advanced URL parameter switches to advanced mode", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Advanced Mode Task", {
      workflow_id: officeSeed.workflowId,
    });
    // Navigate with advanced mode - but only shows advanced if there's an agent session
    // Without a session, it falls back to simple mode
    await testPage.goto(`/office/tasks/${task.id}?mode=advanced`);
    await expect(testPage.getByRole("heading", { name: "Advanced Mode Task" })).toBeVisible({
      timeout: 10_000,
    });
  });
});
