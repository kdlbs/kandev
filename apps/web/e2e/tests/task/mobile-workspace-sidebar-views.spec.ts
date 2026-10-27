import { test, expect } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import {
  expectDefaultWorkspaceViewSortAndIndent,
  expectWorkspaceViewSortAndIndent,
  setWorkspaceViewSortAndIndent,
} from "./sidebar-workspace-view-sort-helpers";
import { openPhoneList } from "./mobile-list-helpers";

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
  await openPhoneList(testPage);
  const workspaceATabs = testPage.getByTestId("sidebar-view-chip-row");
  await testPage.getByTestId("mobile-topbar-page-context").tap();
  const options = testPage.getByRole("dialog", { name: "View options" });
  const editor = options.getByTestId("mobile-list-view-editor");
  await editor.getByTestId("mobile-sidebar-new-view").tap();
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
  await expect(options).toBeHidden();
  await expect(
    workspaceATabs.getByTestId("sidebar-view-chip").filter({ hasText: "Phone A view" }),
  ).toHaveAttribute("data-active", "true");

  await mobile.mobileMenuButton.tap();
  await testPage.getByTestId("mobile-workspace-trigger").tap();
  await testPage.getByTestId(`mobile-workspace-item-${other.id}`).tap();
  await testPage.goto("/tasks");
  const workspaceBTabs = testPage.getByTestId("sidebar-view-chip-row");
  await expect(workspaceBTabs).toBeVisible();
  await expect(workspaceBTabs.getByTestId("sidebar-view-chip")).toHaveCount(1);
  await expect(workspaceBTabs.getByTestId("sidebar-view-chip")).toContainText("All tasks");
  await testPage.getByTestId("mobile-topbar-page-context").tap();
  const otherOptions = testPage.getByRole("dialog", { name: "View options" });
  const otherEditor = otherOptions.getByTestId("mobile-list-view-editor");
  await otherEditor.getByTestId("mobile-sidebar-new-view").tap();
  await otherEditor.getByTestId("view-rename-input").fill("Phone B view");
  await otherEditor.getByTestId("view-rename-confirm").tap();
  await expect
    .poll(async () =>
      (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[other.id].views.some(
        (view) => view.name === "Phone B view",
      ),
    )
    .toBe(true);
  await testPage.keyboard.press("Escape");
  await expect(
    workspaceBTabs.getByTestId("sidebar-view-chip").filter({ hasText: "Phone B view" }),
  ).toHaveAttribute("data-active", "true");
  await testPage.reload();
  await mobile.mobileMenuButton.tap();
  await testPage.getByTestId("mobile-workspace-trigger").tap();
  await testPage.getByTestId(`mobile-workspace-item-${seedData.workspaceId}`).tap();
  await testPage.goto("/tasks");
  const restoredTabs = testPage.getByTestId("sidebar-view-chip-row");
  await expect(
    restoredTabs.getByTestId("sidebar-view-chip").filter({ hasText: "Phone A view" }),
  ).toHaveAttribute("data-active", "true");
  await expect(
    restoredTabs.getByTestId("sidebar-view-chip").filter({ hasText: "Phone B view" }),
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
