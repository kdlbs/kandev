import { test, expect } from "../../fixtures/test-base";
import { SidebarTasksPage } from "../../pages/sidebar-tasks-page";
import { openBlockedTaskLoadingState } from "./task-loading-state-helpers";

test.describe("Task loading state", () => {
  test("shows a spinner instead of a blank task detail pane while task data loads", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const unblockTaskDetailRequest = await openBlockedTaskLoadingState({
      testPage,
      apiClient,
      seedData,
      title: "Task Loading State Anchor",
      unresolvedTaskId: "unresolved-task-detail-desktop",
    });

    try {
      await expect(testPage.getByTestId("task-loading-state")).toBeVisible({ timeout: 10_000 });
      await expect(testPage.getByText("Loading task...")).toBeVisible();
    } finally {
      await unblockTaskDetailRequest();
    }
  });

  test("keeps sibling tasks available after a missing task route", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const sibling = await apiClient.createTask(seedData.workspaceId, "Missing route sibling", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    await testPage.goto(`/t/missing-task-route-desktop?workspaceId=${seedData.workspaceId}`);
    await expect(testPage.getByTestId("task-load-error-state")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByTestId("task-unavailable-overview-link")).toBeVisible();

    const sidebar = new SidebarTasksPage(testPage);
    await expect(sidebar.row(sibling.id)).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText("No tasks yet.", { exact: true })).toHaveCount(0);

    await sidebar.plainClick(sibling.id);
    await expect(testPage).toHaveURL(new RegExp(`/t/${sibling.id}$`));
    await expect(testPage.getByTestId("task-load-error-state")).toBeHidden({ timeout: 10_000 });
  });
});
