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
 *   - Saved views CRUD (save-as, rename, delete)
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
    await filters.setGroup("None");
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
    const toggle = filters.popover.getByTestId("sort-direction-toggle");
    const initial = await toggle.getAttribute("data-direction");
    await toggle.click();
    const flipped = await toggle.getAttribute("data-direction");
    expect(flipped).not.toBe(initial);
  });
});

test.describe("Sidebar filter — saved views CRUD", () => {
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
