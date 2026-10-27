import { expect, test } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { seedIdleSession } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import { expectTouchControl } from "../../helpers/control-sizing";

test("mobile task drawer retains rows while workspace context refresh recovers", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  let readPhase: "healthy" | "unavailable" | "recovering" = "healthy";
  const recovery = Promise.withResolvers<void>();
  await testPage.route("**/api/v1/**", async (route) => {
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
    if (readPhase !== "unavailable") {
      if (readPhase === "recovering") await recovery.promise;
      await route.continue();
      return;
    }
    await route.fulfill({
      status: 503,
      headers: { "Retry-After": "120" },
      contentType: "application/json",
      json: { error: "workspace context temporarily unavailable" },
    });
  });

  const session = await seedIdleSession(testPage, apiClient, seedData, "Mobile retained task");
  await expect(testPage.getByTestId("mobile-task-layout")).toBeVisible();

  readPhase = "unavailable";
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
  await waitForFiniteAnimations(drawer);
  const retry = drawer
    .getByTestId("sidebar-task-load-error")
    .getByRole("button", { name: "Retry", exact: true });
  await expect(retry).toBeVisible();
  await expectTouchControl(retry, "mobile sidebar retry");

  readPhase = "recovering";
  const recoveredWorkflowRead = waitForHttp(testPage, "GET", /^\/api\/v1\/workflows$/, {
    predicate: (response) => response.ok(),
  });
  try {
    await retry.tap();
    await expect(drawer.getByText("Mobile retained task", { exact: true })).toBeVisible();
  } finally {
    recovery.resolve();
  }
  await recoveredWorkflowRead;
  await expect(drawer.getByTestId("sidebar-task-load-error")).toHaveCount(0);
  await expect(drawer.getByText("Mobile retained task", { exact: true })).toBeVisible();
});

test("mobile task drawer surfaces a failed sidebar page and recovers", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  const task = await apiClient.createTask(seedData.workspaceId, "Mobile page retained task", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await apiClient.archiveTask(task.id);
  await apiClient.saveUserSettings({
    sidebar_view_state: {
      workspace_id: seedData.workspaceId,
      views: [
        {
          id: "cold-archive",
          name: "Cold archive",
          filters: [{ id: "archive", dimension: "archived", op: "is", value: true }],
          sort: { key: "title", direction: "asc" },
          group: "none",
          collapsed_groups: [],
        },
      ],
      active_view_id: "cold-archive",
      draft: null,
    },
  });
  let sidebarPageUnavailable = true;
  await testPage.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    if (
      sidebarPageUnavailable &&
      route.request().method() === "POST" &&
      url.pathname === `/api/v1/workspaces/${seedData.workspaceId}/sidebar/query`
    ) {
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        json: { error: "sidebar page temporarily unavailable" },
      });
      return;
    }
    await route.continue();
  });

  const workflowListLoaded = waitForHttp(testPage, "GET", /^\/api\/v1\/workflows$/, {
    predicate: (response) => response.ok(),
  });
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(testPage.getByTestId("mobile-task-layout")).toBeVisible();
  await workflowListLoaded;

  const drawer = testPage.getByRole("dialog", { name: "Tasks", exact: true });
  const failedSidebarPageRead = waitForHttp(
    testPage,
    "POST",
    new RegExp(`^/api/v1/workspaces/${seedData.workspaceId}/sidebar/query$`),
    { predicate: (response) => response.status() === 503 },
  );
  await testPage.getByTestId("mobile-task-picker-trigger").tap();
  await expect(drawer).toBeVisible();
  await failedSidebarPageRead;
  await expect(drawer.getByTestId("sidebar-task-page-load-error")).toBeVisible();
  await expect(drawer.getByText("No tasks yet.", { exact: true })).toHaveCount(0);

  await waitForFiniteAnimations(drawer);
  const retry = drawer
    .getByTestId("sidebar-task-page-load-error")
    .getByRole("button", { name: "Retry", exact: true });
  await expect(retry).toBeVisible();
  await retry.scrollIntoViewIfNeeded();
  await expectTouchControl(retry, "mobile sidebar page retry");
  await testPage.screenshot({ path: testInfo.outputPath("sidebar-page-retry-mobile.png") });

  sidebarPageUnavailable = false;
  const recoveredSidebarPageRead = waitForHttp(
    testPage,
    "POST",
    new RegExp(`^/api/v1/workspaces/${seedData.workspaceId}/sidebar/query$`),
    { predicate: (response) => response.ok() },
  );
  await retry.tap();
  await recoveredSidebarPageRead;
  await expect(drawer.getByTestId("sidebar-task-page-load-error")).toHaveCount(0);
  await expect(drawer.getByText("Mobile page retained task", { exact: true })).toBeVisible();
});
