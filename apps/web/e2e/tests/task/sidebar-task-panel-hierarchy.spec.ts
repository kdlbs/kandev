import { test, expect } from "../../fixtures/test-base";
import {
  seedNavigationTaskPanel,
  seedWorkflowGroupPanel,
  expectWorkflowGroupHierarchy,
  expectStateGroupHeaders,
} from "../../helpers/navigation-hierarchy";
import { SidebarFilterPopoverPage } from "../../pages/sidebar-filter-popover";

// @covers AC-UI-NAV-HIERARCHY-002.1 AC-UI-NAV-HIERARCHY-002.2 AC-UI-NAV-HIERARCHY-002.3 AC-UI-NAV-HIERARCHY-002.4 AC-UI-NAV-HIERARCHY-002.5
test("status rows collapse by keyboard and saved filters remain visible after reload", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await seedNavigationTaskPanel(apiClient, seedData);
  await testPage.goto("/tasks");
  const sidebar = testPage.getByTestId("app-sidebar");
  const groups = sidebar.getByTestId("sidebar-group-header");
  await expectStateGroupHeaders(sidebar);
  const complete = groups.filter({ hasText: "Completed" });
  await expect(complete).toHaveCount(1);
  await complete.focus();
  await testPage.keyboard.press("Enter");
  await expect(complete).toHaveAttribute("aria-expanded", "false");
  await expect(sidebar.getByTestId("sidebar-task-item")).toHaveCount(3);
  await complete.press("Enter");
  await expect(sidebar.getByTestId("sidebar-task-item")).toHaveCount(4);
  const filters = new SidebarFilterPopoverPage(testPage);
  await filters.addFilterRow();
  await filters.setClauseDimension(0, "Title");
  await filters.setClauseTextValue(0, "TODO:");
  await filters.saveAs("To plan");
  await filters.close();
  await expect(sidebar.getByTestId("sidebar-active-filter-indicator")).toBeVisible();
  await expect(sidebar.getByTestId("sidebar-filter-gear-indicator")).toHaveCount(0);
  await expect(sidebar.getByTestId("sidebar-task-item")).toHaveCount(1);
  await testPage.reload();
  await expect(sidebar.getByTestId("sidebar-active-filter-indicator")).toBeVisible();
  const row = sidebar.getByTestId("sidebar-task-item");
  const bounds = (await sidebar.boundingBox())!;
  const rowBounds = (await row.boundingBox())!;
  expect(rowBounds.x).toBeGreaterThan(bounds.x);
  expect(rowBounds.x + rowBounds.width).toBeLessThan(bounds.x + bounds.width);
  await testPage.screenshot({ path: test.info().outputPath("task-panel-filtered.png") });
});

test("workflow step groups indent tasks and retain nested collapse", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const { settings } = await apiClient.getUserSettings();
  const previousView = settings.sidebar_views_by_workspace[seedData.workspaceId];
  try {
    const { parent, child } = await seedWorkflowGroupPanel(apiClient, seedData);
    await testPage.goto("/tasks");
    const sidebar = testPage.getByTestId("app-sidebar");
    const group = sidebar.locator(
      `[data-testid="sidebar-group"][data-group-key="${seedData.startStepId}"]`,
    );
    await expect(sidebar.getByTestId("sidebar-group-header")).toHaveCount(2);
    await expectWorkflowGroupHierarchy(group);
    const header = group.getByTestId("sidebar-group-header");
    const before = testPage.url();
    await header.focus();
    await testPage.keyboard.press("Enter");
    await expect(header).toHaveAttribute("aria-expanded", "false");
    await expect(group.getByTestId("sidebar-task-item")).toHaveCount(0);
    await expect(testPage).toHaveURL(before);
    await header.press("Enter");
    await expect(group.getByTestId("sidebar-task-item")).toHaveCount(2);
    const subtasks = group.locator(
      `[data-testid="sidebar-subtask-toggle"][data-task-id="${parent.task_id}"]`,
    );
    await subtasks.click();
    await expect(group.getByText("Check recovery actions", { exact: true })).toHaveCount(0);
    await expect(testPage).toHaveURL(before);
    await subtasks.click();
    await group.getByText("Check recovery actions", { exact: true }).click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${child.task_id}`));
  } finally {
    await apiClient.saveUserSettings({
      sidebar_view_state: {
        workspace_id: seedData.workspaceId,
        ...(previousView ?? { views: [], active_view_id: "", draft: null }),
      },
    });
  }
});
