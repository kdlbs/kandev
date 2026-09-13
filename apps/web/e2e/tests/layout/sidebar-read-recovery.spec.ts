import { expect, test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import type { Page } from "@playwright/test";

async function failWorkspaceContextReads(
  testPage: Page,
  workspaceId: string,
  shouldFail: () => boolean,
) {
  await testPage.route("**/api/v1/**", async (route) => {
    if (!shouldFail()) {
      await route.continue();
      return;
    }
    const url = new URL(route.request().url());
    const isWorkflowList =
      url.pathname === "/api/v1/workflows" && url.searchParams.get("workspace_id") === workspaceId;
    const isRepositoryList = url.pathname === `/api/v1/workspaces/${workspaceId}/repositories`;
    const isStepList = url.pathname === `/api/v1/workspaces/${workspaceId}/workflow-steps`;
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
}

test.describe("Sidebar workspace context recovery", () => {
  test("retains same-workspace tasks during a failed refresh and recovers on retry", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Retained sidebar task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCard(task.id)).toBeVisible({ timeout: 10_000 });

    let readsUnavailable = true;
    await failWorkspaceContextReads(testPage, seedData.workspaceId, () => readsUnavailable);
    await testPage.goto(`/stats?workspaceId=${seedData.workspaceId}`);

    const sidebar = testPage.getByTestId("app-sidebar").getByTestId("task-sidebar");
    await expect(sidebar.getByText("Retained sidebar task", { exact: true })).toBeVisible({
      timeout: 10_000,
    });
    await expect(sidebar.getByTestId("sidebar-task-load-error")).toBeVisible({ timeout: 10_000 });
    await expect(sidebar.getByText("No tasks yet.", { exact: true })).toHaveCount(0);

    readsUnavailable = false;
    await sidebar.getByRole("button", { name: "Retry", exact: true }).click();
    await expect(sidebar.getByTestId("sidebar-task-load-error")).toHaveCount(0, {
      timeout: 10_000,
    });
    await expect(sidebar.getByText("Retained sidebar task", { exact: true })).toBeVisible();
  });

  test("surfaces a failed workflow snapshot and recovers without reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.createTask(seedData.workspaceId, "Snapshot retained task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    let snapshotUnavailable = true;
    let workflowListRequests = 0;
    await testPage.route("**/api/v1/**", async (route) => {
      const url = new URL(route.request().url());
      if (
        url.pathname === "/api/v1/workflows" &&
        url.searchParams.get("workspace_id") === seedData.workspaceId
      ) {
        workflowListRequests += 1;
      }
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

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const sidebar = testPage.getByTestId("app-sidebar").getByTestId("task-sidebar");
    await expect(sidebar.getByTestId("sidebar-task-load-error")).toBeVisible({ timeout: 10_000 });
    expect(workflowListRequests).toBeGreaterThan(0);
    await expect(sidebar.getByText("No tasks yet.", { exact: true })).toHaveCount(0);

    snapshotUnavailable = false;
    await sidebar.getByRole("button", { name: "Retry", exact: true }).click();
    await expect(sidebar.getByTestId("sidebar-task-load-error")).toHaveCount(0, {
      timeout: 10_000,
    });
    await expect(sidebar.getByText("Snapshot retained task", { exact: true })).toBeVisible();
  });
});
