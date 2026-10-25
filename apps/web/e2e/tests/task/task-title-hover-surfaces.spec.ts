import { test, expect } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";

/**
 * The title preview is intentionally a Kanban-only affordance. These tests
 * keep the non-Kanban surfaces from regressing back to title/subtask popovers.
 */
const PARENT_TITLE = "Surface hover parent with a title long enough to clamp";
const CHILD_ONE_TITLE = "Surface child one with its own long title";
const CHILD_TWO_TITLE = "Surface child two";

async function seedParentWithSubtasks(apiClient: ApiClient, seedData: SeedData) {
  await apiClient.saveUserSettings({
    workspace_id: seedData.workspaceId,
    enable_preview_on_click: false,
  });
  const parent = await apiClient.createTask(seedData.workspaceId, PARENT_TITLE, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  await apiClient.createTask(seedData.workspaceId, CHILD_ONE_TITLE, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
    parent_id: parent.id,
    workspace_mode: "inherit_parent",
  });
  await apiClient.createTask(seedData.workspaceId, CHILD_TWO_TITLE, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
    parent_id: parent.id,
    workspace_mode: "inherit_parent",
  });
}

async function assertNoTitlePreview(testPage: Page) {
  await dwell(
    testPage,
    300,
    "negative-assertion",
    "non-Kanban task titles must not open a title preview",
  );
  await expect(testPage.getByTestId("task-title-hover-card")).toHaveCount(0);
}

test.describe("Task title hover card on non-Kanban surfaces", () => {
  test("the /tasks rich list does not show a title preview for parent tasks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await seedParentWithSubtasks(apiClient, seedData);

    await testPage.goto("/tasks");
    await testPage.waitForLoadState("networkidle");

    const parentRow = testPage
      .getByTestId("tasks-list")
      .getByTestId("tasks-list-row")
      .filter({ hasText: PARENT_TITLE })
      .first();
    await expect(parentRow).toBeVisible({ timeout: 45_000 });

    await parentRow.getByTestId("tasks-list-row-title").hover();
    await assertNoTitlePreview(testPage);
  });

  test("the AppSidebar task row does not show a title preview for parent tasks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await seedParentWithSubtasks(apiClient, seedData);

    await testPage.goto("/tasks");
    await testPage.waitForLoadState("networkidle");

    const sidebarRow = testPage
      .getByTestId("app-sidebar")
      .getByTestId("sidebar-task-item")
      .filter({ hasText: PARENT_TITLE })
      .first();
    await expect(sidebarRow).toBeVisible({ timeout: 45_000 });

    await sidebarRow.getByText(PARENT_TITLE, { exact: false }).first().hover();
    await assertNoTitlePreview(testPage);
  });
});
