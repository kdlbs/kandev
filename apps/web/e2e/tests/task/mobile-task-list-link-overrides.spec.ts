import { test, expect } from "../../fixtures/test-base";
import { repositorySlug } from "../../../lib/repository-slug";
import { openPhoneList, savePhoneWorkspaceView } from "./mobile-list-helpers";
import {
  readPreviousSidebarViewState,
  restoreSidebarViewState,
} from "./sidebar-running-first-activity-sort-helpers";

test("link group and sort overrides control row metadata without changing the saved view", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const alpha = await apiClient.createTask(seedData.workspaceId, "Alpha link sort", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  const zulu = await apiClient.createTask(seedData.workspaceId, "Zulu link sort", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  const previous = await readPreviousSidebarViewState(apiClient, seedData.workspaceId);
  const savedView = {
    id: "phone-repository-view-" + Date.now(),
    name: "Phone repository view",
    filters: [],
    sort: { key: "lastActivityAt", direction: "desc" },
    group: "repository",
    group_indent: true,
    collapsed_groups: [],
  };
  await savePhoneWorkspaceView(apiClient, seedData.workspaceId, savedView);
  const savedState = await readPreviousSidebarViewState(apiClient, seedData.workspaceId);

  try {
    await openPhoneList(testPage);
    await testPage.goto("/tasks?workspace=" + seedData.workspaceId + "&group=none&sort=title_asc");
    const list = testPage.getByTestId("mobile-tasks-list-scroll");
    const rows = list.locator("[data-task-row-id]");
    await expect(rows).toHaveCount(2);
    await expect(rows.nth(0)).toHaveAttribute("data-task-row-id", alpha.id);
    await expect(rows.nth(1)).toHaveAttribute("data-task-row-id", zulu.id);
    await expect(list.getByTestId("sidebar-task-repository")).toHaveCount(2);
    expect(await readPreviousSidebarViewState(apiClient, seedData.workspaceId)).toEqual(savedState);
  } finally {
    await restoreSidebarViewState(apiClient, seedData.workspaceId, previous);
  }
});

test("workflow link grouping labels rows by the requested group, not the saved executor group", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.createTask(seedData.workspaceId, "Phone state override", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await apiClient.updateTaskState(task.id, "IN_PROGRESS");
  const workflowName = (await apiClient.listWorkflows(seedData.workspaceId)).workflows.find(
    (workflow) => workflow.id === seedData.workflowId,
  )!.name;
  const previous = await readPreviousSidebarViewState(apiClient, seedData.workspaceId);
  const savedView = {
    id: "phone-executor-view-" + Date.now(),
    name: "Phone executor view",
    filters: [],
    sort: { key: "state", direction: "asc" },
    group: "executorType",
    group_indent: true,
    collapsed_groups: [],
  };
  await savePhoneWorkspaceView(apiClient, seedData.workspaceId, savedView);
  const savedState = await readPreviousSidebarViewState(apiClient, seedData.workspaceId);

  try {
    await openPhoneList(testPage);
    await testPage.goto("/tasks?workspace=" + seedData.workspaceId + "&group=workflow");
    const header = testPage
      .getByTestId("mobile-tasks-list-scroll")
      .getByTestId("sidebar-group-header");
    await expect(header).toHaveCount(1);
    await expect(header).toContainText(workflowName);
    await header.tap();
    await expect(header).toHaveAttribute("aria-expanded", "false");
    expect(await readPreviousSidebarViewState(apiClient, seedData.workspaceId)).toEqual(savedState);
  } finally {
    await restoreSidebarViewState(apiClient, seedData.workspaceId, previous);
  }
});

test("temporary workflow grouping clears saved collapsed groups without changing the view", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.createTask(seedData.workspaceId, "Phone expanded link result", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const previous = await readPreviousSidebarViewState(apiClient, seedData.workspaceId);
  const savedView = {
    id: "phone-collapsed-workflow-view-" + Date.now(),
    name: "Collapsed workflow view",
    filters: [],
    sort: { key: "state", direction: "asc" },
    group: "workflow",
    group_indent: true,
    collapsed_groups: [seedData.workflowId],
  };
  await savePhoneWorkspaceView(apiClient, seedData.workspaceId, savedView);
  const savedState = await readPreviousSidebarViewState(apiClient, seedData.workspaceId);

  try {
    await openPhoneList(testPage);
    await testPage.goto(`/tasks?workspace=${seedData.workspaceId}&group=workflow`);
    const list = testPage.getByTestId("mobile-tasks-list-scroll");
    const header = list.getByTestId("sidebar-group-header");
    await expect(header).toHaveAttribute("aria-expanded", "true");
    await expect(
      list.getByTestId("sidebar-task-item").filter({ hasText: task.title }),
    ).toBeVisible();
    expect(await readPreviousSidebarViewState(apiClient, seedData.workspaceId)).toEqual(savedState);
  } finally {
    await restoreSidebarViewState(apiClient, seedData.workspaceId, previous);
  }
});

test("desktop repository IDs in phone links resolve to canonical filter values", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.createTask(seedData.workspaceId, "Phone repository link result", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  const { repositories } = await apiClient.listRepositories(seedData.workspaceId);
  const repository = repositories.find((item) => item.id === seedData.repositoryId)!;
  const slug = repositorySlug(repository);
  const previous = await readPreviousSidebarViewState(apiClient, seedData.workspaceId);
  const savedView = {
    id: "phone-different-repository-view-" + Date.now(),
    name: "Different repository view",
    filters: [
      { id: "different-repository", dimension: "repository", op: "is", value: "other/repo" },
    ],
    sort: { key: "state", direction: "asc" },
    group: "none",
    group_indent: true,
    collapsed_groups: [],
  };
  await savePhoneWorkspaceView(apiClient, seedData.workspaceId, savedView);
  const savedState = await readPreviousSidebarViewState(apiClient, seedData.workspaceId);

  try {
    await openPhoneList(testPage);
    await testPage.goto(
      `/tasks?workspace=${seedData.workspaceId}&repository=${seedData.repositoryId}`,
    );
    await expect(testPage.getByTestId("mobile-list-search-row")).toBeVisible();
    await expect(testPage.getByTestId("mobile-list-link-scope")).toBeVisible();
    const list = testPage.getByTestId("mobile-tasks-list-scroll");
    await expect(
      list.getByTestId("sidebar-task-item").filter({ hasText: task.title }),
    ).toBeVisible();
    await expect(list.getByTestId("sidebar-task-repository")).toContainText(slug);
    expect(await readPreviousSidebarViewState(apiClient, seedData.workspaceId)).toEqual(savedState);
  } finally {
    await restoreSidebarViewState(apiClient, seedData.workspaceId, previous);
  }
});
