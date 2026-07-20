import { test, expect } from "../../fixtures/test-base";

// Regression: unarchiving from the task-detail top bar must clear the archived
// UI *in place*. The detail view seeds `archived_at` from a one-shot fetchTask
// that only re-fetches when the task id changes, so the successful lifecycle
// request must explicitly clear that local detail state.
test("unarchiving from the detail top bar clears the archived UI in place", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.createTask(seedData.workspaceId, "Unarchive in place", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await apiClient.archiveTask(task.id);

  await testPage.goto(`/t/${task.id}`);

  const unarchiveButton = testPage.getByTestId("task-unarchive-button");
  await expect(unarchiveButton).toBeVisible();

  const responsePromise = testPage.waitForResponse((response) =>
    response.url().endsWith(`/api/v1/tasks/${task.id}/unarchive`),
  );
  await unarchiveButton.click();
  await responsePromise;

  await expect(testPage.getByText("Task unarchived")).toBeVisible();
  // The Unarchive button only renders while isArchived is true, so its removal
  // proves the detail view observed the unarchive without a navigation/refetch.
  await expect(unarchiveButton).toHaveCount(0);
  // No redirect away — unarchive keeps the user on the same task route.
  await expect(testPage).toHaveURL((url) => url.pathname === `/t/${task.id}`);
});
