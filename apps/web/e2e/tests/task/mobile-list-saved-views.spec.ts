import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { openPhoneList } from "./mobile-list-helpers";
import {
  expectSidebarRootOrder,
  readPreviousSidebarColorPatch,
  readPreviousSidebarViewState,
  restoreSidebarSortColors,
  restoreSidebarViewState,
  saveSidebarRunningRankView,
} from "./sidebar-running-first-activity-sort-helpers";

test.describe.configure({ retries: 0 });

const DEFAULT_VIEW = {
  id: "view-all-tasks",
  name: "All tasks",
  filters: [],
  sort: { key: "state", direction: "asc" },
  group: "none",
  group_indent: true,
  collapsed_groups: [],
};

async function saveWorkspaceView(
  apiClient: ApiClient,
  workspaceId: string,
  views: Array<Record<string, unknown>>,
  activeViewId: string,
) {
  await apiClient.saveUserSettings({
    sidebar_view_state: {
      workspace_id: workspaceId,
      views,
      active_view_id: activeViewId,
      draft: null,
    },
  });
}

test("saved views: phone List selects the saved view for its active workspace", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const token = `phone-view-${Date.now()}`;
  const viewA = {
    ...DEFAULT_VIEW,
    id: `${token}-a`,
    name: "Workspace A mobile view",
    filters: [{ id: `${token}-filter-a`, dimension: "titleMatch", op: "matches", value: token }],
  };
  const viewB = { ...DEFAULT_VIEW, id: `${token}-b`, name: "Workspace B mobile view" };
  const taskA = await apiClient.createTask(seedData.workspaceId, `${token} A result`, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const taskOther = await apiClient.createTask(seedData.workspaceId, `unrelated-${Date.now()}`, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const previousSettings = await apiClient.getUserSettings();
  const previousA = await readPreviousSidebarViewState(apiClient, seedData.workspaceId);
  const other = await apiClient.createWorkspace(`${token} other workspace`);
  await apiClient.createWorkflow(other.id, `${token} workflow`, "simple");
  const previousB = await readPreviousSidebarViewState(apiClient, other.id);
  const previousWorkspaceId = previousSettings.settings.workspace_id;
  const previousLayout = previousSettings.settings.kanban_view_mode;

  await saveWorkspaceView(apiClient, seedData.workspaceId, [DEFAULT_VIEW, viewA], viewA.id);
  await saveWorkspaceView(apiClient, other.id, [DEFAULT_VIEW, viewB], viewB.id);

  try {
    await openPhoneList(testPage);
    const tabs = testPage.getByTestId("sidebar-view-chip-row");
    const chipA = tabs.getByTestId("sidebar-view-chip").filter({ hasText: viewA.name });
    await expect(chipA).toHaveAttribute("data-active", "true");
    await expect(tabs.getByTestId("sidebar-view-chip").filter({ hasText: viewB.name })).toHaveCount(
      0,
    );
    await expect(testPage.getByTestId("tasks-list-archive-modes")).toHaveCount(0);
    await expect(
      testPage.getByTestId("sidebar-task-item").filter({ hasText: taskA.title }),
    ).toBeVisible();
    await expect(
      testPage.getByTestId("sidebar-task-item").filter({ hasText: taskOther.title }),
    ).toHaveCount(0);

    const allTasksChip = tabs.getByTestId("sidebar-view-chip").filter({ hasText: "All tasks" });
    await allTasksChip.tap();
    await expect(
      testPage.getByTestId("sidebar-task-item").filter({ hasText: taskOther.title }),
    ).toBeVisible();
    await chipA.tap();
    await expect(
      testPage.getByTestId("sidebar-task-item").filter({ hasText: taskOther.title }),
    ).toHaveCount(0);

    await testPage.getByTestId("app-nav-trigger").tap();
    await testPage.getByTestId("mobile-workspace-trigger").tap();
    await testPage.getByTestId(`mobile-workspace-item-${other.id}`).tap();
    await testPage.goto("/tasks");
    await expect(testPage.getByTestId("sidebar-view-chip-row")).toBeVisible();
    const workspaceBTabs = testPage.getByTestId("sidebar-view-chip-row");
    await expect(
      workspaceBTabs.getByTestId("sidebar-view-chip").filter({ hasText: viewB.name }),
    ).toBeVisible();
    await expect(
      workspaceBTabs.getByTestId("sidebar-view-chip").filter({ hasText: viewA.name }),
    ).toHaveCount(0);
    await expect(
      workspaceBTabs.getByTestId("sidebar-view-chip").filter({ hasText: viewB.name }),
    ).toHaveAttribute("data-active", "true");
  } finally {
    await restoreSidebarViewState(apiClient, seedData.workspaceId, previousA);
    await restoreSidebarViewState(apiClient, other.id, previousB);
    await apiClient.saveUserSettings({
      workspace_id: previousWorkspaceId ?? seedData.workspaceId,
      ...(previousLayout === undefined ? {} : { kanban_view_mode: previousLayout }),
    });
  }
});

test("view options: the left List control edits the selected saved view and discards its draft", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const previous = await readPreviousSidebarViewState(apiClient, seedData.workspaceId);
  const view = {
    ...DEFAULT_VIEW,
    id: `phone-editor-${Date.now()}`,
    name: "Phone editor view",
    sort: { key: "lastActivityAt", direction: "desc" },
  };
  await saveWorkspaceView(apiClient, seedData.workspaceId, [DEFAULT_VIEW, view], view.id);
  const savedState = await readPreviousSidebarViewState(apiClient, seedData.workspaceId);

  try {
    await openPhoneList(testPage);
    await testPage.getByTestId("mobile-topbar-page-context").tap();
    const options = testPage.getByRole("dialog", { name: "View options" });
    const editor = options.getByTestId("mobile-list-view-editor");
    await expect(editor.getByTestId("sidebar-filter-active-view-name")).toHaveText(view.name);
    await expect(editor.getByTestId("mobile-list-view-editor-scroll")).toHaveCSS(
      "overflow-y",
      "auto",
    );
    await editor.getByTestId("sidebar-sort-settings-toggle").tap();
    await expect(editor.getByTestId("sidebar-sort-chain-editor")).toBeVisible();

    await editor.getByTestId("sidebar-group-settings-toggle").tap();
    await editor.getByTestId("group-key-select").tap();
    await testPage.getByRole("option", { name: "Workflow step", exact: true }).tap();
    await expect(editor.getByTestId("mobile-list-view-draft-footer")).toBeVisible();
    await editor.getByTestId("view-discard-button").tap();
    await expect(editor.getByTestId("mobile-list-view-draft-footer")).toHaveCount(0);
    await expect
      .poll(() => readPreviousSidebarViewState(apiClient, seedData.workspaceId))
      .toEqual(savedState);
  } finally {
    await restoreSidebarViewState(apiClient, seedData.workspaceId, previous);
  }
});

test("search toggle: phone List reveals a focused temporary search without saved writes", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const token = `phone-search-${Date.now()}`;
  const matchingTask = await apiClient.createTask(seedData.workspaceId, `${token} match`, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const otherTask = await apiClient.createTask(seedData.workspaceId, `${token} other`, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const previousViews = await readPreviousSidebarViewState(apiClient, seedData.workspaceId);
  try {
    await openPhoneList(testPage);
    await expect(testPage.getByTestId("mobile-list-search-row")).toHaveCount(0);
    await expect(testPage.getByTestId("mobile-list-search-toggle")).toBeVisible();
    await testPage.getByTestId("mobile-list-search-toggle").tap();
    const search = testPage.getByTestId("mobile-list-search-input");
    await expect(search).toBeFocused();
    await search.fill(token + " match");
    await expect(
      testPage.getByTestId("sidebar-task-item").filter({ hasText: matchingTask.title }),
    ).toBeVisible();
    await expect(
      testPage.getByTestId("sidebar-task-item").filter({ hasText: otherTask.title }),
    ).toHaveCount(0);
    expect(await readPreviousSidebarViewState(apiClient, seedData.workspaceId)).toEqual(
      previousViews,
    );
  } finally {
    await restoreSidebarViewState(apiClient, seedData.workspaceId, previousViews);
  }
});

test("sort chain: phone List applies Running, Red, Orange, then activity before paging", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  const token = `phone-chain-${Date.now()}`;
  const previousViews = await readPreviousSidebarViewState(apiClient, seedData.workspaceId);
  const previousColors = await readPreviousSidebarColorPatch(apiClient);
  const taskOptions = {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  };
  const runningRed = await apiClient.createTask(
    seedData.workspaceId,
    `${token} running red`,
    taskOptions,
  );
  const runningOrange = await apiClient.createTask(
    seedData.workspaceId,
    `${token} running orange`,
    taskOptions,
  );
  const idleRedOld = await apiClient.createTask(
    seedData.workspaceId,
    `${token} idle red old`,
    taskOptions,
  );
  const idleRedNew = await apiClient.createTask(
    seedData.workspaceId,
    `${token} idle red new`,
    taskOptions,
  );
  const idleOrange = await apiClient.createTask(
    seedData.workspaceId,
    `${token} idle orange`,
    taskOptions,
  );
  for (let offset = 0; offset < 100; offset += 10) {
    await Promise.all(
      Array.from({ length: 10 }, (_, index) =>
        apiClient.seedTask(seedData.workspaceId, `${token} filler ${offset + index}`, taskOptions),
      ),
    );
  }
  const { session_id: redSession } = await apiClient.seedTaskSession(runningRed.id, {
    state: "RUNNING",
    startedAt: new Date(Date.now() - 60_000).toISOString(),
  });
  const { session_id: orangeSession } = await apiClient.seedTaskSession(runningOrange.id, {
    state: "RUNNING",
    startedAt: new Date(Date.now() - 60_000).toISOString(),
  });
  await apiClient.setPrimarySession(redSession);
  await apiClient.setPrimarySession(orangeSession);
  await apiClient.saveUserSettings({
    sidebar_task_color_patch: {
      colors: {
        [runningRed.id]: "red",
        [runningOrange.id]: "orange",
        [idleRedOld.id]: "red",
        [idleRedNew.id]: "red",
        [idleOrange.id]: "orange",
      },
      if_missing: false,
    },
  });
  await apiClient.updateTaskTitle(idleRedNew.id, `${token} idle red newest`);
  const viewId = `${token}-view`;
  await saveSidebarRunningRankView(apiClient, seedData, viewId, token);

  try {
    await openPhoneList(testPage);
    const list = testPage.getByTestId("mobile-tasks-list-scroll");
    await expect(list.getByTestId("sidebar-page-controls")).toContainText("Page 1 of 2");
    await expectSidebarRootOrder(
      list,
      [runningRed.id, runningOrange.id, idleRedNew.id, idleRedOld.id, idleOrange.id],
      [runningRed.id, runningOrange.id, idleRedNew.id, idleRedOld.id, idleOrange.id],
    );
    await list.getByRole("button", { name: "Next" }).click();
    await expect(list.getByTestId("sidebar-page-controls")).toContainText("Page 2 of 2");
    await expect(
      list
        .locator("[data-task-row-id]")
        .filter({ hasText: `${token} filler` })
        .first(),
    ).toBeVisible();
  } finally {
    await restoreSidebarViewState(apiClient, seedData.workspaceId, previousViews);
    await restoreSidebarSortColors(
      apiClient,
      [runningRed.id, runningOrange.id, idleRedOld.id, idleRedNew.id, idleOrange.id],
      previousColors,
    );
  }
});

test("pagination: phone List keeps task browsing bounded and row navigation usable", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  const token = `phone-pagination-${Date.now()}`;
  const previousViews = await readPreviousSidebarViewState(apiClient, seedData.workspaceId);
  for (let offset = 0; offset < 100; offset += 10) {
    await Promise.all(
      Array.from({ length: 10 }, (_, index) =>
        apiClient.seedTask(seedData.workspaceId, `${token} row ${offset + index}`, {
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
        }),
      ),
    );
  }
  const navigationTask = await apiClient.createTask(seedData.workspaceId, `${token} navigation`, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const { session_id: navigationSession } = await apiClient.seedTaskSession(navigationTask.id, {
    state: "COMPLETED",
    completedAt: new Date().toISOString(),
  });
  await apiClient.setPrimarySession(navigationSession);
  const viewId = `${token}-view`;
  await saveWorkspaceView(
    apiClient,
    seedData.workspaceId,
    [
      {
        ...DEFAULT_VIEW,
        id: viewId,
        name: `${token} saved view`,
        filters: [{ id: `${token}-filter`, dimension: "titleMatch", op: "matches", value: token }],
        sort: { key: "lastActivityAt", direction: "desc" },
      },
    ],
    viewId,
  );

  try {
    await openPhoneList(testPage);
    const list = testPage.getByTestId("mobile-tasks-list-scroll");
    await expect(list.getByTestId("sidebar-page-controls")).toContainText("Page 1 of 2");
    const pagination = list.getByTestId("sidebar-page-controls");
    await pagination.getByRole("button", { name: "Next" }).click();
    await expect(pagination).toContainText("Page 2 of 2");
    await pagination.getByRole("button", { name: "Previous" }).click();
    await expect(pagination).toContainText("Page 1 of 2");
    await list
      .getByTestId("sidebar-task-item")
      .filter({ hasText: `${token} navigation` })
      .click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${navigationTask.id}$`));
  } finally {
    await restoreSidebarViewState(apiClient, seedData.workspaceId, previousViews);
  }
});
