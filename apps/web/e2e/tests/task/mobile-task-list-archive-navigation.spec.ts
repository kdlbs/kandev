import { test, expect } from "../../fixtures/test-base";
import { openPhoneList, savePhoneWorkspaceView } from "./mobile-list-helpers";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  readPreviousSidebarViewState,
  restoreSidebarViewState,
} from "./sidebar-running-first-activity-sort-helpers";

test("browses an archived saved view by touch and opens an archived task", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const active = await apiClient.createTask(seedData.workspaceId, "Phone active result", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const archived = await apiClient.createTask(seedData.workspaceId, "Phone archived result", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await apiClient.archiveTask(archived.id);
  const previous = await readPreviousSidebarViewState(apiClient, seedData.workspaceId);
  const archivedView = {
    id: `phone-archive-${Date.now()}`,
    name: "Phone archived tasks",
    filters: [{ id: "phone-archived-filter", dimension: "archived", op: "is", value: true }],
    sort: { key: "state", direction: "asc" },
    group: "none",
    group_indent: true,
    collapsed_groups: [],
  };
  await savePhoneWorkspaceView(apiClient, seedData.workspaceId, archivedView);

  try {
    await openPhoneList(testPage);
    const list = testPage.getByTestId("mobile-tasks-list-scroll");
    const tabs = testPage.getByTestId("sidebar-view-chip-row");
    const archivedChip = tabs.getByTestId("sidebar-view-chip").filter({
      hasText: archivedView.name,
    });
    await expect(archivedChip).toHaveAttribute("data-active", "true");
    await expect(
      list.getByTestId("sidebar-task-item").filter({ hasText: archived.title }),
    ).toBeVisible();
    await expect(
      list.getByTestId("sidebar-task-item").filter({ hasText: active.title }),
    ).toHaveCount(0);
    await assertNoDocumentHorizontalOverflow(testPage);

    await list.getByTestId("sidebar-task-item").filter({ hasText: archived.title }).tap();
    await expect(testPage).toHaveURL(new RegExp(`/t/${archived.id}$`));
    await expect(testPage.getByTestId("task-unarchive-button")).toBeVisible();
  } finally {
    await restoreSidebarViewState(apiClient, seedData.workspaceId, previous);
  }
});

test("phone archive links replace the selected saved view archive scope", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const active = await apiClient.createTask(seedData.workspaceId, "Phone active link result", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const archived = await apiClient.createTask(seedData.workspaceId, "Phone archived link result", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await apiClient.archiveTask(archived.id);
  const previous = await readPreviousSidebarViewState(apiClient, seedData.workspaceId);
  const scopedView = {
    id: `phone-archive-scope-${Date.now()}`,
    name: "Phone active workflow",
    filters: [
      { id: "phone-workflow-filter", dimension: "workflow", op: "is", value: seedData.workflowId },
      { id: "phone-active-filter", dimension: "archived", op: "is", value: false },
    ],
    sort: { key: "state", direction: "asc" },
    group: "none",
    group_indent: true,
    collapsed_groups: [],
  };
  await savePhoneWorkspaceView(apiClient, seedData.workspaceId, scopedView);

  try {
    const list = testPage.getByTestId("mobile-tasks-list-scroll");
    const navigateToArchive = (archive: string) =>
      testPage.goto(`/tasks?workspace=${seedData.workspaceId}&archive=${archive}`);

    await navigateToArchive("archived");
    await expect(
      list.getByTestId("sidebar-task-item").filter({ hasText: archived.title }),
    ).toBeVisible();
    await expect(
      list.getByTestId("sidebar-task-item").filter({ hasText: active.title }),
    ).toHaveCount(0);

    await navigateToArchive("all");
    await expect(
      list.getByTestId("sidebar-task-item").filter({ hasText: archived.title }),
    ).toBeVisible();
    await expect(
      list.getByTestId("sidebar-task-item").filter({ hasText: active.title }),
    ).toBeVisible();

    await navigateToArchive("active");
    await expect(
      list.getByTestId("sidebar-task-item").filter({ hasText: active.title }),
    ).toBeVisible();
    await expect(
      list.getByTestId("sidebar-task-item").filter({ hasText: archived.title }),
    ).toHaveCount(0);
  } finally {
    await restoreSidebarViewState(apiClient, seedData.workspaceId, previous);
  }
});
