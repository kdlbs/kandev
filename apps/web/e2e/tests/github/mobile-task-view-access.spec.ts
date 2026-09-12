import { test, expect } from "../../fixtures/test-base";
import { MobileGitHubPage } from "../../pages/mobile-github-page";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

// @covers AC-UI-MOBILE-TASK-VIEWS-001.1
// @covers AC-UI-MOBILE-TASK-VIEWS-001.2
test("GitHub app navigation opens shared task views and preserves browser Back", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("test-user");
  const { settings } = await apiClient.getUserSettings();
  expect(typeof settings.sidebar_active_view_id).toBe("string");
  const view = {
    id: "github-mobile-task-view",
    name: "My mobile tasks",
    filters: [],
    sort: { key: "updatedAt", direction: "desc" },
    group: "none",
    collapsedGroups: [],
  };
  await apiClient.saveUserSettings({ sidebar_views: [view], sidebar_active_view_id: view.id });
  const task = await apiClient.seedTask(seedData.workspaceId, "Task reachable from GitHub", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  try {
    const github = new MobileGitHubPage(testPage);
    await github.goto();
    const opener = testPage.getByTestId("app-nav-trigger");
    await opener.tap();
    await testPage
      .getByTestId("app-nav-sheet")
      .getByRole("button", { name: "Task views", exact: true })
      .tap();
    const drawer = testPage.getByRole("dialog", { name: "Tasks", exact: true });
    await expect(drawer).toBeVisible();
    await expect(testPage.getByTestId("app-nav-sheet")).toBeHidden();
    await expect(testPage.locator('[role="dialog"]:visible')).toHaveCount(1);
    await expect(
      drawer.getByTestId("sidebar-view-chip").filter({ hasText: view.name }),
    ).toHaveAttribute("data-active", "true");
    await expect(drawer.getByTestId("sidebar-new-view")).toBeVisible();
    await testPage.keyboard.press("Escape");
    await expect(drawer).toBeHidden();
    await expect(opener).toBeFocused();
    await opener.tap();
    await testPage
      .getByTestId("app-nav-sheet")
      .getByRole("button", { name: "Task views", exact: true })
      .tap();
    await drawer.locator(`[data-task-row-id="${task.task_id}"]`).tap();
    await expect(testPage).toHaveURL(new RegExp(`/t/${task.task_id}`));
    await testPage.goBack();
    await expect(testPage).toHaveURL(/\/github$/);
    await expect(github.mobileMenuButton).toBeVisible();
    // The task remains selected in shared state, but GitHub has no task-only providers.
    await opener.tap();
    await testPage
      .getByTestId("app-nav-sheet")
      .getByRole("button", { name: "Task views", exact: true })
      .tap();
    await expect(drawer.getByTestId("sidebar-filter-bar")).toBeVisible();
    await drawer.getByRole("button", { name: "New", exact: true }).tap();
    await expect(drawer).toBeHidden();
    await expect(testPage.getByRole("dialog")).toBeVisible();
    await expect(testPage.getByRole("dialog")).not.toHaveAccessibleName("Tasks");
    const titleInput = testPage.getByTestId("create-task-dialog").getByTestId("task-title-input");
    await titleInput.fill("Draft survives phone rotation");
    await testPage.setViewportSize({ width: 851, height: 393 });
    await expect(titleInput).toHaveValue("Draft survives phone rotation");
  } finally {
    await apiClient.saveUserSettings({
      sidebar_views: (settings.sidebar_views ?? []) as unknown[],
      sidebar_active_view_id: settings.sidebar_active_view_id as string,
    });
  }
});

// @covers AC-UI-MOBILE-TASK-VIEWS-001.1
// @covers AC-UI-MOBILE-TASK-VIEWS-001.2
test("Kanban navigation can open empty task views and return focus", async ({ testPage }) => {
  const kanban = new MobileKanbanPage(testPage);
  await kanban.goto();
  await kanban.mobileMenuButton.tap();
  await kanban.menuCard.getByRole("button", { name: "Task views", exact: true }).tap();
  const drawer = testPage.getByRole("dialog", { name: "Tasks", exact: true });
  await expect(drawer.getByTestId("sidebar-filter-bar")).toBeVisible();
  await expect(drawer.locator('[data-slot="task-switcher-empty-state"]')).toBeVisible();
  await expect(kanban.menuCard).toBeHidden();
  await testPage.keyboard.press("Escape");
  await expect(drawer).toBeHidden();
  await expect(kanban.mobileMenuButton).toBeFocused();
});
