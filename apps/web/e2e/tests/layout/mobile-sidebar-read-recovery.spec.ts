import { expect, test } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";

test("mobile task drawer retains rows while workspace context refresh recovers", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.createTask(seedData.workspaceId, "Mobile retained task", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });

  let readsUnavailable = false;
  await testPage.route("**/api/v1/**", async (route) => {
    if (!readsUnavailable) {
      await route.continue();
      return;
    }
    const url = new URL(route.request().url());
    const isWorkflowList =
      url.pathname === "/api/v1/workflows" &&
      url.searchParams.get("workspace_id") === seedData.workspaceId;
    const isRepositoryList =
      url.pathname === `/api/v1/workspaces/${seedData.workspaceId}/repositories`;
    const isStepList = url.pathname === `/api/v1/workspaces/${seedData.workspaceId}/workflow-steps`;
    if (!isWorkflowList && !isRepositoryList && !isStepList) {
      await route.continue();
      return;
    }
    await route.fulfill({
      status: 503,
      contentType: "application/json",
      json: { error: "workspace context temporarily unavailable" },
    });
  });

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(testPage.getByTestId("mobile-task-layout")).toBeVisible();

  readsUnavailable = true;
  const failedWorkflowRead = waitForHttp(testPage, "GET", /^\/api\/v1\/workflows$/, {
    predicate: (response) => response.status() === 503,
  });
  await testPage.reload();
  await failedWorkflowRead;
  await session.waitForLoad();
  const drawer = testPage.getByRole("dialog", { name: "Tasks", exact: true });
  await testPage.getByTestId("mobile-task-picker-trigger").tap();
  await expect(drawer).toBeVisible();
  await expect(drawer.getByText("Mobile retained task", { exact: true })).toBeVisible();
  await expect(drawer.getByTestId("sidebar-task-load-error")).toBeVisible();
  const retry = drawer.getByRole("button", { name: "Retry", exact: true });
  const retryBox = await retry.boundingBox();
  if (!retryBox) throw new Error("mobile sidebar retry control is not visible");
  expect(retryBox.height).toBeGreaterThanOrEqual(44);

  readsUnavailable = false;
  const recoveredWorkflowRead = waitForHttp(testPage, "GET", /^\/api\/v1\/workflows$/, {
    predicate: (response) => response.ok(),
  });
  await retry.tap();
  await recoveredWorkflowRead;
  await expect(drawer.getByTestId("sidebar-task-load-error")).toHaveCount(0);
  await expect(drawer.getByText("Mobile retained task", { exact: true })).toBeVisible();
});

test("mobile task drawer surfaces a failed workflow snapshot and recovers", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.createTask(seedData.workspaceId, "Mobile snapshot retained task", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  let snapshotUnavailable = true;
  await testPage.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    if (
      snapshotUnavailable &&
      url.pathname === `/api/v1/workflows/${seedData.workflowId}/snapshot`
    ) {
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        json: { error: "workflow snapshot temporarily unavailable" },
      });
      return;
    }
    await route.continue();
  });

  const workflowListLoaded = waitForHttp(testPage, "GET", /^\/api\/v1\/workflows$/, {
    predicate: (response) => response.ok(),
  });
  const failedSnapshotRead = waitForHttp(
    testPage,
    "GET",
    new RegExp(`^/api/v1/workflows/${seedData.workflowId}/snapshot$`),
    { predicate: (response) => response.status() === 503 },
  );
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(testPage.getByTestId("mobile-task-layout")).toBeVisible();
  await workflowListLoaded;
  await failedSnapshotRead;

  const drawer = testPage.getByRole("dialog", { name: "Tasks", exact: true });
  await testPage.getByTestId("mobile-task-picker-trigger").tap();
  await expect(drawer).toBeVisible();
  await expect(drawer.getByTestId("sidebar-task-load-error")).toBeVisible();
  await expect(drawer.getByText("No tasks yet.", { exact: true })).toHaveCount(0);

  const retry = drawer.getByRole("button", { name: "Retry", exact: true });
  const retryBox = await retry.boundingBox();
  if (!retryBox) throw new Error("mobile sidebar retry control is not visible");
  expect(retryBox.height).toBeGreaterThanOrEqual(44);

  snapshotUnavailable = false;
  const recoveredSnapshotRead = waitForHttp(
    testPage,
    "GET",
    new RegExp(`^/api/v1/workflows/${seedData.workflowId}/snapshot$`),
    { predicate: (response) => response.ok() },
  );
  await retry.tap();
  await recoveredSnapshotRead;
  await expect(drawer.getByTestId("sidebar-task-load-error")).toHaveCount(0);
  await expect(drawer.getByText("Mobile snapshot retained task", { exact: true })).toBeVisible();
});
