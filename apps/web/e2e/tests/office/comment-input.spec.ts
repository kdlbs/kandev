import { test, expect } from "../../fixtures/office-fixture";

test.describe("Comment input", () => {
  test("comment input is visible on issue detail", async ({ testPage, apiClient, officeSeed }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Comment Input Task", {
      workflow_id: officeSeed.workflowId,
    });
    await testPage.goto(`/office/tasks/${task.id}`);
    await expect(testPage.getByRole("heading", { name: "Comment Input Task" })).toBeVisible({
      timeout: 10_000,
    });
    await expect(testPage.getByPlaceholder("Add a comment")).toBeVisible({ timeout: 5_000 });
  });

  test("submit a comment via UI", async ({ testPage, apiClient, officeSeed }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Submit Comment Task", {
      workflow_id: officeSeed.workflowId,
    });
    await testPage.goto(`/office/tasks/${task.id}`);
    await expect(testPage.getByRole("heading", { name: "Submit Comment Task" })).toBeVisible({
      timeout: 10_000,
    });

    const input = testPage.getByPlaceholder("Add a comment");
    await expect(input).toBeVisible({ timeout: 5_000 });
    await input.fill("Hello from E2E test");
    await input.press("Enter");

    // Comment should appear after submission
    await expect(testPage.getByText("Hello from E2E test")).toBeVisible({ timeout: 10_000 });
  });
});
