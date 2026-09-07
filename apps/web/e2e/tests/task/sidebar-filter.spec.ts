/**
 * E2E tests for the composable sidebar filter / sort / group system + saved views.
 *
 * After the unified AppSidebar overhaul, the filter UI lives in the AppSidebar
 * Tasks-section header as the `TasksViewPicker` (a view-picker dropdown +
 * filter gear) rather than the legacy `sidebar-filter-bar`. The picker is
 * visible whenever the Tasks section is expanded, which it is by default and on
 * `/t/` routes. The `SidebarFilterPopoverPage` page object encapsulates the new
 * surface (see its docstring). Saved-view chips now live inside the picker's
 * dropdown menu instead of an always-visible chip row.
 *
 * Coverage:
 *   - Gear popover open/close
 *   - Default "All tasks" view is seeded for new users
 *   - Filter add/remove, negation, live list update
 *   - Sort + direction toggle
 *   - Group-by (repository, state, none)
 *   - Direct saved-view creation plus save-as, rename, and delete
 *   - Persistence across reload
 *   - Draft semantics + discard
 *   - Last-view deletion guard
 *
 * NOTE: Drag-to-reorder of saved views was removed by the overhaul — the views
 * are now dropdown-menu items, not a sortable chip row. The "view ordering"
 * tests that depended on dragging are dropped / adapted accordingly (see the
 * "view ordering" describe block).
 */
import path from "node:path";
import fs from "node:fs";
import { execSync } from "node:child_process";
import { test, expect } from "../../fixtures/test-base";
import { makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";
import { SidebarFilterPopoverPage } from "../../pages/sidebar-filter-popover";

async function openWithSeed(
  testPage: import("@playwright/test").Page,
  apiClient: import("../../helpers/api-client").ApiClient,
  seedData: import("../../fixtures/test-base").SeedData,
  taskTitles: string[],
): Promise<{ session: SessionPage; filters: SidebarFilterPopoverPage }> {
  for (const title of taskTitles) {
    await apiClient.createTask(seedData.workspaceId, title, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
  }
  const navTask = await apiClient.createTask(seedData.workspaceId, "Sidebar Filter Nav", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await testPage.goto(`/t/${navTask.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.sidebar).toBeVisible({ timeout: 10_000 });
  const filters = new SidebarFilterPopoverPage(testPage);
  await expect(filters.bar).toBeVisible();
  return { session, filters };
}

async function saveTitleView(
  filters: SidebarFilterPopoverPage,
  name: string,
  titleFilter: string,
): Promise<void> {
  await filters.close();
  await filters.selectViewByName("All tasks");
  await filters.expectActiveViewChip("All tasks");
  await filters.addFilterRow();
  await filters.setClauseDimension(0, "Title");
  await filters.setClauseTextValue(0, titleFilter);
  await filters.saveAs(name);
  await filters.expectActiveViewChip(name);
  await filters.close();
}

async function taskRowOrder(surface: import("@playwright/test").Locator, taskIds: string[]) {
  const rows = await surface.locator("[data-task-row-id]").all();
  const order: string[] = [];
  for (const row of rows) {
    const id = await row.getAttribute("data-task-row-id");
    if (id && taskIds.includes(id)) order.push(id);
  }
  return order;
}

async function taskActivityAt(
  apiClient: import("../../helpers/api-client").ApiClient,
  workspaceId: string,
  taskId: string,
): Promise<string | null> {
  const result = await apiClient.listTasks(workspaceId);
  return result.tasks.find((task) => task.id === taskId)?.status_summary?.last_activity_at ?? null;
}

test.describe("Sidebar filter bar — popover basics", () => {
  test("gear opens popover; ESC closes it", async ({ testPage, apiClient, seedData }) => {
    const { filters } = await openWithSeed(testPage, apiClient, seedData, ["Basics Task"]);
    await filters.open();
    await expect(filters.popover).toBeVisible();
    await filters.close();
    await expect(filters.popover).toBeHidden();
  });

  test("default 'All tasks' view is seeded for new users", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { filters } = await openWithSeed(testPage, apiClient, seedData, ["Chip Task"]);
    await filters.openViewPicker();
    const chips = filters.chipMenu.getByTestId("sidebar-view-chip");
    await expect(chips).toHaveCount(1);
    await expect(chips.filter({ hasText: "All tasks" })).toBeVisible();
    await filters.closeViewPicker();

    await filters.addFilterRow();
    await filters.setClauseDimension(0, "Archived");
    await expect
      .poll(async () => {
        const { settings } = await apiClient.getUserSettings();
        const draft = settings.sidebar_draft as { base_view_id?: string } | null | undefined;
        return draft?.base_view_id ?? null;
      })
      .toBe("view-all-tasks");
    await expect(testPage.getByText("Sidebar views", { exact: true })).toHaveCount(0);

    await testPage.reload();
    await new SessionPage(testPage).waitForLoad();
    await new SidebarFilterPopoverPage(testPage).expectActiveViewChip("All tasks");
  });

  test("switching chips updates active state and persists across reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { filters } = await openWithSeed(testPage, apiClient, seedData, ["Persist Task"]);
    await filters.addFilterRow();
    await filters.setClauseDimension(0, "Title");
    await filters.setClauseTextValue(0, "persist");
    await filters.saveAs("Persist View");
    await filters.expectActiveViewChip("Persist View");
    await expect
      .poll(async () => {
        const { settings } = await apiClient.getUserSettings();
        return (settings.sidebar_views as Array<{ name?: string }> | undefined)?.some(
          (view) => view.name === "Persist View",
        );
      })
      .toBe(true);

    await testPage.reload();
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const filters2 = new SidebarFilterPopoverPage(testPage);
    await filters2.expectActiveViewChip("Persist View");
  });
});

test.describe("Sidebar filter — view ordering", () => {
  // The "dragged view order persists to settings and survives reload" test was
  // DELETED in the AppSidebar overhaul: saved views moved from a horizontal,
  // drag-sortable `sidebar-view-chip-row` into the `TasksViewPicker` dropdown
  // menu. Dropdown menu items are not sortable, so there is no longer a
  // drag-to-reorder affordance to exercise. The underlying store action
  // (reorderSidebarViews) still exists but has no UI entry point, so an E2E
  // test would have nothing to drive. Re-add coverage if a reorder affordance
  // returns to the picker.

  test("appended views select, delete, and append normally", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { session, filters } = await openWithSeed(testPage, apiClient, seedData, [
      "Select alpha task",
      "Select beta task",
      "Select gamma task",
    ]);
    await saveTitleView(filters, "Alpha View", "alpha");
    await saveTitleView(filters, "Beta View", "beta");
    await saveTitleView(filters, "Gamma View", "gamma");
    // Views append in creation order after the default "All tasks".
    await filters.expectChipOrder(["All tasks", "Alpha View", "Beta View", "Gamma View"]);

    await filters.selectViewByName("Alpha View");
    await filters.expectActiveViewChip("Alpha View");
    await expect(session.sidebar.getByText("Select alpha task")).toBeVisible();
    await expect(session.sidebar.getByText("Select beta task")).toHaveCount(0);

    await filters.selectViewByName("Beta View");
    await filters.open();
    await filters.deleteActiveView();
    await filters.expectChipOrder(["All tasks", "Alpha View", "Gamma View"]);
    // Deleting the active view falls back to the first remaining view.
    await filters.expectActiveViewChip("All tasks");

    await saveTitleView(filters, "Delta View", "delta");
    await filters.expectChipOrder(["All tasks", "Alpha View", "Gamma View", "Delta View"]);
    await filters.expectActiveViewChip("Delta View");
  });
});

test.describe("Sidebar filter — filtering", () => {
  test("offers archived as a filter dimension and loads archived rows", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const archivedTask = await apiClient.createTask(seedData.workspaceId, "Archived filter task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.archiveTask(archivedTask.id);
    const { session, filters } = await openWithSeed(testPage, apiClient, seedData, [
      "Filter options task",
    ]);
    await filters.addFilterRow();
    await filters.clauseRow(0).getByTestId("filter-dimension-select").click();

    await expect(testPage.getByRole("option", { name: "Archived", exact: true })).toBeVisible();
    await testPage.getByRole("option", { name: "Archived", exact: true }).click();
    await filters.close();
    await expect(testPage.getByText("Archived filter task")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText("Archived", { exact: true })).toBeVisible();
    await expect(testPage.getByText("Filter options task")).toHaveCount(0);

    const liveArchivedTask = await apiClient.createTask(
      seedData.workspaceId,
      "Live archived filter task",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      },
    );
    await apiClient.archiveTask(liveArchivedTask.id);
    await expect(session.sidebar.getByText("Live archived filter task")).toHaveCount(1, {
      timeout: 10_000,
    });

    await session.sidebar.getByText("Archived filter task", { exact: true }).click();
    await expect(testPage).toHaveURL((url) => url.pathname === `/t/${archivedTask.id}`);
    await expect(testPage.getByTestId("task-unarchive-button")).toBeVisible({ timeout: 10_000 });
  });

  test("adding a title filter narrows the list live", async ({ testPage, apiClient, seedData }) => {
    const { session, filters } = await openWithSeed(testPage, apiClient, seedData, [
      "Fix auth bug",
      "Update deps",
      "Refactor auth",
    ]);
    await filters.addFilterRow();
    await filters.setClauseDimension(0, "Title");
    await filters.setClauseTextValue(0, "auth");
    await filters.close();

    await expect(session.sidebar.getByText("Fix auth bug")).toBeVisible();
    await expect(session.sidebar.getByText("Refactor auth")).toBeVisible();
    await expect(session.sidebar.getByText("Update deps")).toHaveCount(0);
  });

  test("negation: title 'does not contain' hides matching tasks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { session, filters } = await openWithSeed(testPage, apiClient, seedData, [
      "Fix auth",
      "Update deps",
    ]);
    await filters.addFilterRow();
    await filters.setClauseDimension(0, "Title");
    await filters.setClauseOp(0, "does not contain");
    await filters.setClauseTextValue(0, "auth");
    await filters.close();

    await expect(session.sidebar.getByText("Update deps")).toBeVisible();
    await expect(session.sidebar.getByText("Fix auth")).toHaveCount(0);
  });

  test("remove clause restores full list", async ({ testPage, apiClient, seedData }) => {
    const { session, filters } = await openWithSeed(testPage, apiClient, seedData, [
      "Keep me",
      "Drop me later",
    ]);
    await filters.addFilterRow();
    await filters.setClauseDimension(0, "Title");
    await filters.setClauseTextValue(0, "keep");
    await filters.close();
    await expect(session.sidebar.getByText("Drop me later")).toHaveCount(0);

    await filters.open();
    await filters.removeClause(0);
    await filters.close();
    await expect(session.sidebar.getByText("Drop me later")).toBeVisible();
  });
});

test.describe("Sidebar filter — group + sort", () => {
  test("Group by none hides group headers", async ({ testPage, apiClient, seedData }) => {
    const { session, filters } = await openWithSeed(testPage, apiClient, seedData, ["One", "Two"]);
    await filters.open();
    await filters.openGroupSettings();
    await filters.popover.getByTestId("group-key-select").click();
    for (const { label, description } of [
      { label: "None", description: "Keep all tasks in one list." },
      { label: "Repository", description: "Separate tasks by repository." },
      { label: "Workflow", description: "Separate tasks by workflow." },
      { label: "Workflow step", description: "Separate tasks by workflow step." },
      { label: "Executor type", description: "Separate tasks by executor type." },
      { label: "State", description: "Separate tasks by state." },
    ]) {
      const option = testPage.getByRole("option", { name: label, exact: true });
      const descriptionId = await option.getAttribute("aria-describedby");
      expect(descriptionId).toBeTruthy();
      await expect(testPage.locator(`[id="${descriptionId}"]`)).toHaveText(description);
    }
    await testPage.getByRole("option", { name: "None", exact: true }).click();
    await filters.close();
    await expect(session.sidebar.locator("[data-testid='sidebar-group-header']")).toHaveCount(0);
  });

  test("Group by state shows state-bucket headers", async ({ testPage, apiClient, seedData }) => {
    const { session, filters } = await openWithSeed(testPage, apiClient, seedData, ["State Task"]);
    await filters.open();
    await filters.setGroup("State");
    await filters.close();
    const headers = session.sidebar.locator("[data-testid='sidebar-group-header']");
    await expect(headers.first()).toBeVisible();
  });

  test("Sort direction toggle flips icon direction", async ({ testPage, apiClient, seedData }) => {
    const { filters } = await openWithSeed(testPage, apiClient, seedData, ["Sort A"]);
    await filters.open();
    await filters.openSortSettings();
    const toggle = filters.popover.getByTestId("sort-direction-toggle");
    const initial = await toggle.getAttribute("data-direction");
    await toggle.click();
    const flipped = await toggle.getAttribute("data-direction");
    expect(flipped).not.toBe(initial);
  });

  test("sorts by last activity, persists, and ignores provider-only refresh", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const oldTask = await apiClient.createTask(seedData.workspaceId, "Activity old", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const newTask = await apiClient.createTask(seedData.workspaceId, "Activity new", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.updateTaskTitle(oldTask.id, "Activity old touched");
    const navTask = await apiClient.createTask(seedData.workspaceId, "Activity sort nav", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await testPage.goto(`/t/${navTask.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const filters = new SidebarFilterPopoverPage(testPage);
    await filters.open();
    await filters.openSortSettings();
    await filters.popover.getByTestId("sort-key-select").click();
    await expect(testPage.getByRole("option", { name: "Updated", exact: true })).toContainText(
      "Last task summary refresh. Background events can change it.",
    );
    await expect(
      testPage.getByRole("option", { name: "Last activity", exact: true }),
    ).toContainText("Last user or agent action. Viewing a task does not change it.");
    await expect(testPage.getByRole("option", { name: "Status", exact: true })).toContainText(
      "Task state, from review to backlog.",
    );
    await expect(testPage.getByRole("option", { name: "Created", exact: true })).toContainText(
      "When the task was created.",
    );
    await expect(testPage.getByRole("option", { name: "Title", exact: true })).toContainText(
      "Task title in alphabetical order.",
    );
    await expect(testPage.getByRole("option", { name: "Custom", exact: true })).toContainText(
      "The manual order you set for tasks.",
    );
    for (const { label, description } of [
      {
        label: "Updated",
        description: "Last task summary refresh. Background events can change it.",
      },
      {
        label: "Last activity",
        description: "Last user or agent action. Viewing a task does not change it.",
      },
      { label: "Status", description: "Task state, from review to backlog." },
      { label: "Created", description: "When the task was created." },
      { label: "Title", description: "Task title in alphabetical order." },
      { label: "Custom", description: "The manual order you set for tasks." },
    ]) {
      const option = testPage.getByRole("option", { name: label, exact: true });
      const descriptionId = await option.getAttribute("aria-describedby");
      expect(descriptionId).toBeTruthy();
      await expect(testPage.locator(`[id="${descriptionId}"]`)).toHaveText(description);
    }
    await testPage.keyboard.press("Escape");
    await filters.setGroup("None");
    await filters.setSort("Last activity", "desc");
    await filters.saveAs("Last activity view");
    await filters.close();

    await expect
      .poll(() => taskRowOrder(session.sidebar, [oldTask.id, newTask.id]))
      .toEqual([oldTask.id, newTask.id]);
    let activityAt: string | null = null;
    await expect
      .poll(async () => {
        activityAt = await taskActivityAt(apiClient, seedData.workspaceId, oldTask.id);
        return activityAt;
      })
      .toBeTruthy();
    const oldRow = session.sidebar.locator(`[data-task-row-id="${oldTask.id}"]`);
    await expect(oldRow.getByTestId("sidebar-task-time")).toHaveAttribute(
      "data-time-value",
      activityAt!,
    );

    await testPage.reload();
    await new SessionPage(testPage).waitForLoad();
    const reloadedFilters = new SidebarFilterPopoverPage(testPage);
    await reloadedFilters.expectActiveViewChip("Last activity view");
    await expect
      .poll(() => taskRowOrder(session.sidebar, [oldTask.id, newTask.id]))
      .toEqual([oldTask.id, newTask.id]);

    await apiClient.mockGitHubAssociateTaskPR({
      task_id: newTask.id,
      workspace_id: seedData.workspaceId,
      repository_id: seedData.repositoryId,
      owner: "activity-owner",
      repo: "activity-repo",
      pr_number: 7,
      pr_url: "https://github.com/activity-owner/activity-repo/pull/7",
      pr_title: "Activity PR",
      head_branch: "activity",
      base_branch: "main",
      author_login: "activity-owner",
      checks_state: "failure",
    });
    await expect
      .poll(async () => {
        const tasks = await apiClient.listTasks(seedData.workspaceId);
        return tasks.tasks.find((task) => task.id === newTask.id)?.status_summary?.pull_request
          ?.aggregate_state;
      })
      .toBe("failure");
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: newTask.id,
      workspace_id: seedData.workspaceId,
      repository_id: seedData.repositoryId,
      owner: "activity-owner",
      repo: "activity-repo",
      pr_number: 7,
      pr_url: "https://github.com/activity-owner/activity-repo/pull/7",
      pr_title: "Activity PR refreshed",
      head_branch: "activity",
      base_branch: "main",
      author_login: "activity-owner",
      checks_state: "success",
    });
    await expect
      .poll(async () => {
        const tasks = await apiClient.listTasks(seedData.workspaceId);
        return tasks.tasks.find((task) => task.id === newTask.id)?.status_summary?.pull_request
          ?.aggregate_state;
      })
      .toBe("passing");
    await expect
      .poll(() => taskRowOrder(session.sidebar, [oldTask.id, newTask.id]))
      .toEqual([oldTask.id, newTask.id]);
  });
});

test.describe("Sidebar filter — saved views CRUD", () => {
  test("direct creation starts from canonical defaults, focuses rename, and persists", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { filters } = await openWithSeed(testPage, apiClient, seedData, ["Direct View Task"]);

    // Make the active source visibly non-default before creating the blank view.
    await filters.addFilterRow();
    await filters.setClauseDimension(0, "Title");
    await filters.setClauseTextValue(0, "source-only");
    await filters.setSort("Updated", "desc");
    await filters.setGroup("State");
    await filters.saveAs("Custom source");
    await filters.close();

    const renameInput = await filters.beginNewView();
    await expect(renameInput).toHaveValue("New view");
    await expect(filters.popover.getByTestId("filter-clause-row")).toHaveCount(0);
    await expect(filters.popover.getByTestId("sort-key-select")).toHaveCount(0);
    await expect(filters.popover.getByTestId("group-key-select")).toHaveCount(0);
    await filters.openSortSettings();
    await expect(filters.popover.getByTestId("sort-key-select")).toContainText("Status");
    await expect(filters.popover.getByTestId("sort-direction-toggle")).toHaveAttribute(
      "data-direction",
      "asc",
    );
    await filters.openGroupSettings();
    await expect(filters.popover.getByTestId("group-key-select")).toContainText("Repository");

    await renameInput.fill("Planning view");
    await filters.popover.getByTestId("view-rename-confirm").click();
    await expect(filters.popover).toBeVisible();
    await expect(filters.popover.getByTestId("sidebar-filter-active-view-name")).toContainText(
      "Planning view",
    );
    await expect
      .poll(async () => {
        const { settings } = await apiClient.getUserSettings();
        return (settings.sidebar_views as Array<{ name?: string }> | undefined)?.some(
          (view) => view.name === "Planning view",
        );
      })
      .toBe(true);

    await testPage.reload();
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await new SidebarFilterPopoverPage(testPage).expectActiveViewChip("Planning view");
  });

  test("cancelling optional rename keeps automatic names and creates the next gap", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { filters } = await openWithSeed(testPage, apiClient, seedData, ["Auto Name Task"]);

    await filters.beginNewView();
    await filters.popover.getByRole("button", { name: "Cancel" }).click();
    await expect(filters.popover.getByTestId("sidebar-filter-active-view-name")).toContainText(
      "New view",
    );
    await filters.close();

    const secondName = await filters.beginNewView();
    await expect(secondName).toHaveValue("New view 2");
    await filters.popover.getByRole("button", { name: "Cancel" }).click();
    await filters.close();
    await filters.expectChipOrder(["All tasks", "New view", "New view 2"]);
  });

  test("direct creation explains dirty-draft and saved-view-limit blocks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { filters } = await openWithSeed(testPage, apiClient, seedData, ["Blocked View Task"]);
    await filters.addFilterRow();
    await filters.close();
    await filters.openViewPicker();
    await expect(filters.newViewAction).toHaveAttribute("aria-disabled", "true");
    await expect(filters.newViewAction).toHaveAttribute("aria-label", /save or discard changes/i);
    await filters.closeViewPicker();
    await filters.open();
    await expect(filters.popover.getByTestId("sidebar-filter-dirty-indicator")).toBeVisible();
    await filters.discard();
    await expect
      .poll(async () => {
        const { settings } = await apiClient.getUserSettings();
        return settings.sidebar_draft ?? null;
      })
      .toBeNull();

    const limitViews = Array.from({ length: 50 }, (_, index) => ({
      id: `limit-view-${index}`,
      name: `Limit view ${index + 1}`,
      filters: [],
      sort: { key: "state", direction: "asc" },
      group: "repository",
      collapsed_groups: [],
    }));
    const response = await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      sidebar_views: limitViews,
      sidebar_active_view_id: limitViews[0].id,
      sidebar_draft: null,
    });
    expect(response.ok).toBe(true);
    await expect
      .poll(async () => {
        const { settings } = await apiClient.getUserSettings();
        return (settings.sidebar_views as unknown[] | undefined)?.length;
      })
      .toBe(50);
    await testPage.reload();
    await new SessionPage(testPage).waitForLoad();
    const reloaded = new SidebarFilterPopoverPage(testPage);
    await reloaded.openViewPicker();
    await reloaded.newViewAction.scrollIntoViewIfNeeded();
    await expect(reloaded.newViewAction).toHaveAttribute("aria-disabled", "true");
    await expect(reloaded.newViewAction).toHaveAttribute("aria-label", /up to 50 views/i);
  });

  test("save-as creates a new chip and selects it", async ({ testPage, apiClient, seedData }) => {
    const { filters } = await openWithSeed(testPage, apiClient, seedData, ["View CRUD Task"]);
    await filters.addFilterRow();
    await filters.setClauseDimension(0, "Title");
    await filters.setClauseTextValue(0, "foo");
    await filters.saveAs("My View");
    await filters.expectActiveViewChip("My View");

    await testPage.reload();
    const session2 = new SessionPage(testPage);
    await session2.waitForLoad();
    const f2 = new SidebarFilterPopoverPage(testPage);
    await f2.openViewPicker();
    await expect(
      f2.chipMenu.getByTestId("sidebar-view-chip").filter({ hasText: "My View" }),
    ).toBeVisible();
  });

  test("delete custom view removes chip and falls back to remaining view", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { filters } = await openWithSeed(testPage, apiClient, seedData, ["Delete View Task"]);
    await filters.addFilterRow();
    await filters.setClauseDimension(0, "Title");
    await filters.setClauseTextValue(0, "zz");
    await filters.saveAs("Ephemeral");
    await filters.expectActiveViewChip("Ephemeral");

    await filters.open();
    await filters.deleteActiveView();
    await filters.close();
    await filters.openViewPicker();
    await expect(
      filters.chipMenu.getByTestId("sidebar-view-chip").filter({ hasText: "Ephemeral" }),
    ).toHaveCount(0);
    await filters.closeViewPicker();
    await filters.expectActiveViewChip("All tasks");
  });

  test("last remaining view cannot be deleted (delete button hidden)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { filters } = await openWithSeed(testPage, apiClient, seedData, ["Last View Task"]);
    await filters.open();
    await expect(filters.popover.getByTestId("view-delete-button")).toHaveCount(0);
  });
});

test.describe("Sidebar filter — repository dimension (#1213)", () => {
  test("filtering by a repository keeps only that repo's tasks (not an empty board)", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    // The default seeded repo is a LOCAL repo (no GitHub provider), named
    // "E2E Repo". Regression for #1213: a local repo's filter option value used
    // to be its full local_path while each task carried the repo *name*, so a
    // saved repository clause matched nothing and the board went empty.

    // A second, distinct local repo so we can prove the clause keeps matches and
    // drops non-matches.
    const repoBName = "Second E2E Repo";
    const repoBDir = path.join(backend.tmpDir, "repos", "e2e-repo-filter-b");
    fs.mkdirSync(repoBDir, { recursive: true });
    const gitEnv = makeGitEnv(backend.tmpDir);
    execSync("git init -b main", { cwd: repoBDir, env: gitEnv });
    execSync('git commit --allow-empty -m "init"', { cwd: repoBDir, env: gitEnv });
    const repoB = await apiClient.createRepository(seedData.workspaceId, repoBDir, "main", {
      name: repoBName,
    });

    await apiClient.createTask(seedData.workspaceId, "Task in repo A", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await apiClient.createTask(seedData.workspaceId, "Task in repo B", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [repoB.id],
    });

    const navTask = await apiClient.createTask(seedData.workspaceId, "Repo Filter Nav", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto(`/t/${navTask.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.sidebar).toBeVisible({ timeout: 10_000 });
    const filters = new SidebarFilterPopoverPage(testPage);
    await expect(filters.bar).toBeVisible();

    // Both tasks visible before filtering.
    await expect(session.sidebar.getByText("Task in repo A")).toBeVisible();
    await expect(session.sidebar.getByText("Task in repo B")).toBeVisible();

    await filters.addFilterRow();
    await filters.setClauseDimension(0, "Repository");
    await filters.setClauseEnumValue(0, "E2E Repo");
    await filters.close();

    // Repo A's task survives (this was empty before the fix); repo B's is hidden.
    await expect(session.sidebar.getByText("Task in repo A")).toBeVisible();
    await expect(session.sidebar.getByText("Task in repo B")).toHaveCount(0);
  });
});

test.describe("Sidebar filter — task-row presentation", () => {
  test("previews, saves, discards, and reloads task row presentation with compact trailing content", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const taskTitle = "Desktop task row layout";
    const secondTaskTitle = "Desktop task row layout second";
    const { session, filters } = await openWithSeed(testPage, apiClient, seedData, [
      taskTitle,
      secondTaskTitle,
    ]);
    const row = session.sidebar
      .getByTestId("sidebar-task-item")
      .filter({ has: testPage.getByText(taskTitle, { exact: true }) });
    const secondRow = session.sidebar
      .getByTestId("sidebar-task-item")
      .filter({ has: testPage.getByText(secondTaskTitle, { exact: true }) });
    await expect(row).toBeVisible();
    await expect(row.getByTestId("sidebar-task-time")).toBeVisible();

    await filters.open();
    await expect(filters.taskRowSettings.getByTestId("task-row-settings-toggle")).toBeVisible();
    await expect(filters.taskRowSettings.getByTestId("task-row-details-toggle")).toHaveCount(0);
    await expect(filters.popover.getByTestId("sidebar-filter-dirty-indicator")).toHaveCount(0);

    await filters.openTaskRowSettings();
    await expect(filters.popover.getByTestId("sidebar-filter-dirty-indicator")).toHaveCount(0);
    await filters.taskRowSettings.getByTestId("task-row-trailing-select").click();
    for (const { label, description } of [
      { label: "Git changes", description: "Show added and removed lines." },
      { label: "Relative time", description: "Show when the task was last updated." },
      {
        label: "Change request status",
        description: "Show the pull request or merge request status.",
      },
      { label: "Nothing", description: "Leave the right side empty." },
    ]) {
      const option = testPage.getByRole("option", { name: label, exact: true });
      const descriptionId = await option.getAttribute("aria-describedby");
      expect(descriptionId).toBeTruthy();
      await expect(testPage.locator(`[id="${descriptionId}"]`)).toHaveText(description);
    }
    await testPage.keyboard.press("Escape");
    await prCapture.screenshot("desktop-task-row-settings", {
      caption: "Desktop task-row presentation settings with the section expanded",
    });
    const pullRequestHandle = filters.taskRowSettings.getByTestId(
      "task-row-detail-handle-pull_request_number",
    );
    const relativeTimeHandle = filters.taskRowSettings.getByTestId(
      "task-row-detail-handle-relative_time",
    );
    const sourceBox = await pullRequestHandle.boundingBox();
    const targetBox = await relativeTimeHandle.boundingBox();
    expect(sourceBox).not.toBeNull();
    expect(targetBox).not.toBeNull();
    await testPage.mouse.move(
      sourceBox!.x + sourceBox!.width / 2,
      sourceBox!.y + sourceBox!.height / 2,
    );
    await testPage.mouse.down();
    await testPage.mouse.move(
      sourceBox!.x + sourceBox!.width / 2,
      sourceBox!.y + sourceBox!.height / 2 + 12,
      { steps: 4 },
    );
    await testPage.mouse.move(targetBox!.x + targetBox!.width / 2, targetBox!.y + 2, {
      steps: 16,
    });
    await testPage.mouse.up();
    await expect
      .poll(() => filters.taskRowDetailOrder())
      .toEqual(["pull_request_number", "relative_time", "repository"]);
    await filters.toggleTaskRowDetail("repository");
    await expect(row.getByTestId("sidebar-task-repository")).toHaveCount(0);

    const detailsToggle = filters.taskRowSettings.getByTestId("task-row-details-toggle");
    await detailsToggle.click();
    await expect(row.getByTestId("sidebar-task-time")).toHaveCount(0);
    await expect(row.getByText(taskTitle, { exact: true })).toBeVisible();
    const compactRowBox = await row.boundingBox();
    const compactTitleBox = await row.getByText(taskTitle, { exact: true }).first().boundingBox();
    expect(compactRowBox).not.toBeNull();
    expect(compactTitleBox).not.toBeNull();
    expect(
      Math.abs(
        compactTitleBox!.y +
          compactTitleBox!.height / 2 -
          (compactRowBox!.y + compactRowBox!.height / 2),
      ),
    ).toBeLessThanOrEqual(1);
    await filters.saveAs("Compact task rows");
    await filters.expectActiveViewChip("Compact task rows");

    const savedSettings = await apiClient.getUserSettings();
    const savedViews = savedSettings.settings.sidebar_views as Array<{
      name?: string;
      task_row?: { details_enabled?: boolean };
    }>;
    expect(savedViews.find((view) => view.name === "Compact task rows")?.task_row).toMatchObject({
      details_enabled: false,
    });

    await filters.setTaskRowTrailing("Relative time");
    await filters.saveOverwrite();
    const trailingTime = row.getByTestId("sidebar-task-trailing-time");
    const secondTrailingTime = secondRow.getByTestId("sidebar-task-trailing-time");
    await expect(trailingTime).toBeVisible();
    await expect(secondTrailingTime).toBeVisible();
    await expect(trailingTime).not.toContainText(/ago|yesterday/i);
    await expect(trailingTime.locator(".sr-only")).toHaveText(/\S/);
    const [timeBox, secondTimeBox] = await Promise.all([
      trailingTime.boundingBox(),
      secondTrailingTime.boundingBox(),
    ]);
    expect(timeBox).not.toBeNull();
    expect(secondTimeBox).not.toBeNull();
    expect(Math.abs(timeBox!.width - secondTimeBox!.width)).toBeLessThanOrEqual(1);

    await filters.openTaskRowSettings();
    await detailsToggle.click();
    await expect(filters.popover.getByTestId("sidebar-filter-dirty-indicator")).toBeVisible();
    await filters.discard();
    await expect(row.getByTestId("sidebar-task-time")).toHaveCount(0);
    await expect(row.getByTestId("sidebar-task-trailing-time")).toBeVisible();
    await filters.close();

    await filters.selectViewByName("All tasks");
    await expect(
      session.sidebar
        .getByTestId("sidebar-task-item")
        .filter({ has: testPage.getByText(taskTitle, { exact: true }) }),
    ).toBeVisible();
    await expect(
      session.sidebar
        .getByTestId("sidebar-task-item")
        .filter({ has: testPage.getByText(taskTitle, { exact: true }) })
        .getByTestId("sidebar-task-time"),
    ).toBeVisible();

    await filters.selectViewByName("Compact task rows");
    await expect
      .poll(async () => {
        const compactRow = session.sidebar
          .getByTestId("sidebar-task-item")
          .filter({ has: testPage.getByText(taskTitle, { exact: true }) });
        return (await compactRow.getByTestId("sidebar-task-trailing-time").count()) === 1;
      })
      .toBe(true);

    await testPage.reload();
    await new SessionPage(testPage).waitForLoad();
    const reloadedFilters = new SidebarFilterPopoverPage(testPage);
    await reloadedFilters.expectActiveViewChip("Compact task rows");
    const reloadedRow = testPage
      .getByTestId("sidebar-task-item")
      .filter({ has: testPage.getByText(taskTitle, { exact: true }) });
    await expect(reloadedRow.getByTestId("sidebar-task-time")).toHaveCount(0);
    await expect(reloadedRow.getByTestId("sidebar-task-trailing-time")).toBeVisible();
    await expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
  });
});

test.describe("Sidebar filter — draft semantics", () => {
  test("dirty indicator appears after edits, clears on discard", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { filters } = await openWithSeed(testPage, apiClient, seedData, ["Draft Task"]);
    await filters.addFilterRow();
    await filters.setClauseDimension(0, "Title");
    await filters.setClauseTextValue(0, "zz");
    await expect(filters.popover.getByTestId("sidebar-filter-dirty-indicator")).toBeVisible();
    await filters.discard();
    await expect(filters.popover.getByTestId("sidebar-filter-dirty-indicator")).toHaveCount(0);
  });
});
