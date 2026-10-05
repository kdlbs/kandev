import { test, expect } from "../../fixtures/test-base";
import {
  seedNavigationTaskPanel,
  seedWorkflowGroupPanel,
  expectWorkflowGroupHierarchy,
  expectStateGroupHeaders,
} from "../../helpers/navigation-hierarchy";
import { expectTouchSquareControl, expectTouchControl } from "../../helpers/control-sizing";

// @covers AC-UI-NAV-HIERARCHY-002.3 AC-UI-NAV-HIERARCHY-002.4 AC-UI-NAV-HIERARCHY-002.5 AC-UI-NAV-HIERARCHY-002.6
test("phone status groups and task actions remain contained in both themes", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await seedNavigationTaskPanel(apiClient, seedData);
  await testPage.goto("/tasks");
  await testPage.getByTestId("app-nav-trigger").tap();
  const menu = testPage.getByTestId("app-nav-sheet");
  await expectStateGroupHeaders(menu);
  for (const width of [393, 767]) {
    await testPage.setViewportSize({ width, height: 851 });
    for (const theme of ["first", "second"]) {
      await menu.getByTestId("mobile-theme-toggle-button").tap();
      const header = menu.getByTestId("sidebar-group-header").first();
      await header.scrollIntoViewIfNeeded();
      await expectTouchControl(header);
      const row = menu.getByTestId("sidebar-task-item").first();
      const action = row.getByRole("button", { name: "Task actions", exact: true });
      await expectTouchSquareControl(action);
      const bounds = (await row.boundingBox())!;
      expect(bounds.x).toBeGreaterThanOrEqual(0);
      expect(bounds.x + bounds.width).toBeLessThanOrEqual(width);
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
      ).toBe(true);
      await testPage.screenshot({
        path: test.info().outputPath(`task-panel-${width}-${theme}.png`),
      });
    }
  }
  const originalURL = testPage.url();
  await menu
    .getByTestId("sidebar-task-item")
    .first()
    .getByRole("button", { name: "Task actions", exact: true })
    .tap();
  await expect(testPage).toHaveURL(originalURL);
  await expect(testPage.getByTestId("task-context-priority")).toBeVisible();
});

test("phone workflow groups retain hierarchy, touch controls, and child navigation", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const { settings } = await apiClient.getUserSettings();
  const previousView = settings.sidebar_views_by_workspace[seedData.workspaceId];
  try {
    const { child } = await seedWorkflowGroupPanel(apiClient, seedData);
    await testPage.goto("/tasks");
    await testPage.getByTestId("app-nav-trigger").tap();
    const menu = testPage.getByTestId("app-nav-sheet");
    const group = menu.locator(
      `[data-testid="sidebar-group"][data-group-key="${seedData.startStepId}"]`,
    );
    const header = group.getByTestId("sidebar-group-header");
    for (const width of [360, 393, 767]) {
      await testPage.setViewportSize({ width, height: 851 });
      await header.scrollIntoViewIfNeeded();
      await expectWorkflowGroupHierarchy(group);
      await expectTouchControl(header);
      const action = group
        .getByTestId("sidebar-task-item")
        .last()
        .getByRole("button", { name: "Task actions", exact: true });
      await expectTouchSquareControl(action);
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
      ).toBe(true);
    }
    await header.tap();
    await expect(group.getByTestId("sidebar-task-item")).toHaveCount(0);
    await header.tap();
    await expect(group.getByTestId("sidebar-task-item")).toHaveCount(2);
    await group.getByText("Check recovery actions", { exact: true }).tap();
    await expect(menu).toBeHidden();
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
