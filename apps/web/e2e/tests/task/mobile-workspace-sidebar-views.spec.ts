import { test, expect } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import {
  expectDefaultWorkspaceViewSortAndIndent,
  expectWorkspaceViewSortAndIndent,
  setWorkspaceViewSortAndIndent,
} from "./sidebar-workspace-view-sort-helpers";

// @covers AC-UI-WORKSPACE-SIDEBAR-VIEWS-001.1, .2, .4, .6
// @covers AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.5 AC-UI-SIDEBAR-GROUP-INDENT-001.4
test("phone task views restore their workspace collection", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  const other = await apiClient.createWorkspace("Phone sidebar workspace");
  await apiClient.createWorkflow(other.id, "Phone sidebar workflow", "simple");
  const mobile = new MobileKanbanPage(testPage);
  await mobile.goto();
  await mobile.mobileMenuButton.tap();
  const drawer = mobile.menuCard;
  await drawer.getByTestId("sidebar-new-view").tap();
  const editor = testPage.getByTestId("sidebar-filter-popover");
  await editor.getByTestId("view-rename-input").fill("Phone A view");
  await editor.getByTestId("view-rename-confirm").tap();
  await expect
    .poll(async () =>
      (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
        seedData.workspaceId
      ].views.some((view) => view.name === "Phone A view"),
    )
    .toBe(true);
  await setWorkspaceViewSortAndIndent(apiClient, seedData.workspaceId, "Phone A view");
  await testPage.keyboard.press("Escape");
  await expect(editor).toBeHidden();
  await testPage.keyboard.press("Escape");
  await expect(drawer).toBeHidden();

  await mobile.mobileMenuButton.tap();
  await testPage.getByTestId("mobile-workspace-trigger").tap();
  await testPage.getByTestId(`mobile-workspace-item-${other.id}`).tap();
  await mobile.mobileMenuButton.tap();
  await expect(drawer.getByTestId("sidebar-view-chip")).toHaveCount(1);
  await expect(drawer.getByTestId("sidebar-view-chip")).toContainText("All tasks");
  await drawer.getByTestId("sidebar-new-view").tap();
  await editor.getByTestId("view-rename-input").fill("Phone B view");
  await editor.getByTestId("view-rename-confirm").tap();
  await expect
    .poll(async () =>
      (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[other.id].views.some(
        (view) => view.name === "Phone B view",
      ),
    )
    .toBe(true);
  await testPage.reload();
  await mobile.mobileMenuButton.tap();
  await testPage.getByTestId("mobile-workspace-trigger").tap();
  await testPage.getByTestId(`mobile-workspace-item-${seedData.workspaceId}`).tap();
  await mobile.mobileMenuButton.tap();
  await expect(
    drawer.getByTestId("sidebar-view-chip").filter({ hasText: "Phone A view" }),
  ).toHaveAttribute("data-active", "true");
  await expect(
    drawer.getByTestId("sidebar-view-chip").filter({ hasText: "Phone B view" }),
  ).toHaveCount(0);
  const { settings } = await apiClient.getUserSettings();
  const viewsA = settings.sidebar_views_by_workspace[seedData.workspaceId].views;
  const viewsB = settings.sidebar_views_by_workspace[other.id].views;
  expectWorkspaceViewSortAndIndent(viewsA.find((view) => view.name === "Phone A view"));
  expectDefaultWorkspaceViewSortAndIndent(viewsB.find((view) => view.name === "Phone B view"));
  expect(
    await testPage.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
  await testPage.screenshot({
    path: testInfo.outputPath("workspace-sidebar-phone.png"),
    animations: "disabled",
  });
});
