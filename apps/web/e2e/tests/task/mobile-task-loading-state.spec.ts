import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";

type E2EStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => {
      tasks: { activeTaskId: string | null };
      setActiveTask: (taskId: string) => void;
    };
  };
};

async function waitForActiveTask(testPage: Page, taskId: string) {
  await testPage.waitForFunction((expectedTaskId) => {
    const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
    return store?.getState().tasks.activeTaskId === expectedTaskId;
  }, taskId);
}

async function switchToUnresolvedTask(testPage: Page, taskId: string) {
  await testPage.evaluate((unresolvedTaskId) => {
    const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
    if (!store) throw new Error("E2E store bridge missing");
    store.getState().setActiveTask(unresolvedTaskId);
  }, taskId);
}

test.describe("Mobile task loading state", () => {
  test("shows a spinner instead of a blank task detail pane while task data loads", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const title = "Mobile Task Loading State Anchor";
    const task = await apiClient.createTask(seedData.workspaceId, title, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    await testPage.goto(`/t/${task.id}`);
    await waitForActiveTask(testPage, task.id);
    await switchToUnresolvedTask(testPage, "unresolved-task-detail-mobile");

    await expect(testPage.getByTestId("task-loading-state")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText("Loading task...")).toBeVisible();
  });
});
