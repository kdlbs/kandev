import { test, expect } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

// @covers AC-UI-WORKSPACE-SIDEBAR-VIEWS-001.1, .2, .4, .6
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
  await mobile.menuCard.getByRole("button", { name: "Task views", exact: true }).tap();
  const drawer = testPage.getByRole("dialog", { name: "Tasks", exact: true });
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
  await testPage.keyboard.press("Escape");
  await expect(editor).toBeHidden();
  await testPage.keyboard.press("Escape");
  await expect(drawer).toBeHidden();

  await mobile.mobileMenuButton.tap();
  await testPage.getByTestId("mobile-workspace-trigger").tap();
  await testPage.getByTestId(`mobile-workspace-item-${other.id}`).tap();
  await mobile.mobileMenuButton.tap();
  await mobile.menuCard.getByRole("button", { name: "Task views", exact: true }).tap();
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
  await mobile.menuCard.getByRole("button", { name: "Task views", exact: true }).tap();
  await expect(
    drawer.getByTestId("sidebar-view-chip").filter({ hasText: "Phone A view" }),
  ).toHaveAttribute("data-active", "true");
  await expect(
    drawer.getByTestId("sidebar-view-chip").filter({ hasText: "Phone B view" }),
  ).toHaveCount(0);
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
