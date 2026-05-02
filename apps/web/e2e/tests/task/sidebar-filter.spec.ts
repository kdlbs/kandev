/**
 * E2E tests for the composable sidebar filter / sort / group system + saved views.
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
 */
import { test, expect } from "../../fixtures/test-base";
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

async function expectStoredSidebarViewOrder(
  apiClient: import("../../helpers/api-client").ApiClient,
  names: string[],
): Promise<void> {
  await expect
    .poll(async () => {
      const response = await apiClient.getUserSettings();
      const views = (response.settings.sidebar_views ?? []) as Array<{ name: string }>;
      return views.map((view) => view.name);
    })
    .toEqual(names);
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
    const chips = filters.chipRow.getByTestId("sidebar-view-chip");
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
  test("dragged view order persists to settings and survives reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { filters } = await openWithSeed(testPage, apiClient, seedData, [
      "Order alpha task",
      "Order beta task",
      "Order gamma task",
    ]);
    await saveTitleView(filters, "Alpha View", "alpha");
    await saveTitleView(filters, "Beta View", "beta");
    await saveTitleView(filters, "Gamma View", "gamma");
    await filters.expectChipOrder(["All tasks", "Alpha View", "Beta View", "Gamma View"]);

    await filters.dragViewBefore("Gamma View", "All tasks");

    const reorderedNames = ["Gamma View", "All tasks", "Alpha View", "Beta View"];
    await filters.expectChipOrder(reorderedNames);
    await expectStoredSidebarViewOrder(apiClient, reorderedNames);

    await testPage.reload();
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const filters2 = new SidebarFilterPopoverPage(testPage);
    await filters2.expectChipOrder(reorderedNames);
    await filters2.expectActiveViewChip("Gamma View");
  });

  test("reordered views still select, delete, and append normally", async ({
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
    await filters.dragViewBefore("Gamma View", "All tasks");
    await filters.expectChipOrder(["Gamma View", "All tasks", "Alpha View", "Beta View"]);

    await filters.selectViewByName("Alpha View");
    await filters.expectActiveViewChip("Alpha View");
    await expect(session.sidebar.getByText("Select alpha task")).toBeVisible();
    await expect(session.sidebar.getByText("Select beta task")).toHaveCount(0);

    await filters.selectViewByName("Beta View");
    await filters.open();
    await filters.deleteActiveView();
    await filters.expectChipOrder(["Gamma View", "All tasks", "Alpha View"]);
    await filters.expectActiveViewChip("Gamma View");

    await saveTitleView(filters, "Delta View", "delta");
    await filters.expectChipOrder(["Gamma View", "All tasks", "Alpha View", "Delta View"]);
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
    const f2 = new SidebarFilterPopoverPage(testPage);
    await expect(
      f2.chipRow.getByTestId("sidebar-view-chip").filter({ hasText: "My View" }),
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
    await expect(
      filters.chipRow.getByTestId("sidebar-view-chip").filter({ hasText: "Ephemeral" }),
    ).toHaveCount(0);
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
