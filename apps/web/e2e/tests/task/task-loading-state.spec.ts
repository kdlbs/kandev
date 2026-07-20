import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { blockTaskDetailRequest, openBlockedTaskLoadingState } from "./task-loading-state-helpers";

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

  test("does not show the full-page spinner when switching to a cached sidebar task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const sourceTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Sidebar Switch Source",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    const targetTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Sidebar Switch Target",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${sourceTask.id}`);
    const session = new SessionPage(testPage);
    await expect(session.sidebarTaskItem("Sidebar Switch Target")).toBeVisible({
      timeout: 15_000,
    });

    const unblockTaskDetailRequest = await blockTaskDetailRequest(testPage, targetTask.id);
    try {
      await session.clickTaskInSidebar("Sidebar Switch Target");
      await expect(testPage).toHaveURL(new RegExp(`/t/${targetTask.id}(?:\\?|$)`), {
        timeout: 15_000,
      });
      await expect(testPage.locator('[aria-current="page"]')).toHaveText("Sidebar Switch Target", {
        timeout: 15_000,
      });
      await expect(testPage.getByTestId("task-loading-state")).toHaveCount(0);
    } finally {
      await unblockTaskDetailRequest();
    }
  });
});
