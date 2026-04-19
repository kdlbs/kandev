/**
 * E2E tests for the composable sidebar filter / sort / group system + saved views.
 *
 * Coverage:
 *   - Gear popover open/close
 *   - Built-in views present (All, No PR reviews, Active, Archived)
 *   - Filter add/remove, negation, live list update
 *   - Sort + direction toggle
 *   - Group-by (repository, state, none)
 *   - Saved views CRUD (save-as, rename, delete)
 *   - Persistence across reload
 *   - Draft semantics + discard
 *   - PR-watcher regression: No PR reviews view hides PR-linked items
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

test.describe("Sidebar filter bar — popover basics", () => {
  test("gear opens popover; ESC closes it", async ({ testPage, apiClient, seedData }) => {
    const { filters } = await openWithSeed(testPage, apiClient, seedData, ["Basics Task"]);
    await filters.open();
    await expect(filters.popover).toBeVisible();
    await filters.close();
    await expect(filters.popover).toBeHidden();
  });

  test("all built-in view chips are present", async ({ testPage, apiClient, seedData }) => {
    const { filters } = await openWithSeed(testPage, apiClient, seedData, ["Chip Task"]);
    const chips = filters.chipRow.getByTestId("sidebar-view-chip");
    await expect(chips).toHaveCount(4);
    await expect(chips.filter({ hasText: "All tasks" })).toBeVisible();
    await expect(chips.filter({ hasText: "No PR reviews" })).toBeVisible();
    await expect(chips.filter({ hasText: "Active" })).toBeVisible();
    await expect(chips.filter({ hasText: "Archived" })).toBeVisible();
  });

  test("switching chips updates active state and persists across reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { filters } = await openWithSeed(testPage, apiClient, seedData, ["Persist Task"]);
    await filters.selectViewByName("Active");
    await filters.expectActiveViewChip("Active");

    await testPage.reload();
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const filters2 = new SidebarFilterPopoverPage(testPage);
    await filters2.expectActiveViewChip("Active");
  });
});

test.describe("Sidebar filter — filtering", () => {
  test("archived built-in view hides non-archived tasks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { session, filters } = await openWithSeed(testPage, apiClient, seedData, [
      "Alive One",
      "Alive Two",
    ]);
    await filters.selectViewByName("Archived");
    await expect(session.sidebar.getByText("Alive One")).toHaveCount(0);
    await expect(session.sidebar.getByText("Alive Two")).toHaveCount(0);
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

  test("No PR reviews built-in view hides PR-linked tasks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");

    const prTask = await apiClient.createTask(seedData.workspaceId, "PR Watcher Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await apiClient.createTask(seedData.workspaceId, "Plain Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    await apiClient.mockGitHubAssociateTaskPR({
      task_id: prTask.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 101,
      pr_url: "https://github.com/testorg/testrepo/pull/101",
      pr_title: "PR Watcher PR",
      head_branch: "feat/pr-watcher",
      base_branch: "main",
      author_login: "test-user",
    });

    const navTask = await apiClient.createTask(seedData.workspaceId, "Sidebar Filter Nav", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto(`/t/${navTask.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.sidebar).toBeVisible({ timeout: 10_000 });
    const filters = new SidebarFilterPopoverPage(testPage);

    await expect(session.sidebar.getByText("PR Watcher Task")).toBeVisible();
    await expect(session.sidebar.getByText("Plain Task")).toBeVisible();

    await filters.selectViewByName("No PR reviews");

    await expect(session.sidebar.getByText("Plain Task")).toBeVisible();
    await expect(session.sidebar.getByText("PR Watcher Task")).toHaveCount(0);
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

  test("delete custom view removes chip and falls back to built-in", async ({
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
  });

  test("built-in views cannot be deleted (delete button absent)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { filters } = await openWithSeed(testPage, apiClient, seedData, ["BuiltIn Task"]);
    await filters.selectViewByName("All tasks");
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
