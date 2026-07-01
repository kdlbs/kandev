import type { Page, Route } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";

type SeedData = {
  workspaceId: string;
  workflowId: string;
  startStepId: string;
  repositoryId: string;
};

type E2EStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => {
      tasks: { activeTaskId: string | null };
      setActiveTask: (taskId: string) => void;
    };
  };
};

type OpenBlockedTaskLoadingStateParams = {
  testPage: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  title: string;
  unresolvedTaskId: string;
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

export async function blockTaskDetailRequest(testPage: Page, taskId: string) {
  const routePattern = `**/api/v1/tasks/${taskId}`;
  let unblock: () => void = () => {};
  const blocked = new Promise<void>((resolve) => {
    unblock = resolve;
  });
  let requestStarted = false;
  let markHandled: () => void = () => {};
  const handled = new Promise<void>((resolve) => {
    markHandled = resolve;
  });

  const handler = async (route: Route) => {
    requestStarted = true;
    await blocked;
    try {
      await route.abort("failed").catch(() => {});
    } finally {
      markHandled();
    }
  };

  await testPage.route(routePattern, handler);

  return async () => {
    unblock();
    if (requestStarted) await handled;
    await testPage.unroute(routePattern, handler);
  };
}

export async function openBlockedTaskLoadingState({
  testPage,
  apiClient,
  seedData,
  title,
  unresolvedTaskId,
}: OpenBlockedTaskLoadingStateParams) {
  const unblockTaskDetailRequest = await blockTaskDetailRequest(testPage, unresolvedTaskId);
  const task = await apiClient.createTask(seedData.workspaceId, title, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });

  await testPage.goto(`/t/${task.id}`);
  await waitForActiveTask(testPage, task.id);
  await switchToUnresolvedTask(testPage, unresolvedTaskId);

  return unblockTaskDetailRequest;
}
