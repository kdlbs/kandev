import { test, expect } from "../../fixtures/test-base";
import { SidebarFilterPopoverPage } from "../../pages/sidebar-filter-popover";

// @covers AC-UI-WORKSPACE-SIDEBAR-VIEWS-001.1, .2, .4, .6
test("desktop task views stay with their workspace after switching and reload", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  const other = await apiClient.createWorkspace("Independent sidebar workspace");
  await apiClient.createWorkflow(other.id, "Independent workflow", "simple");
  await testPage.goto(`/?workspaceId=${seedData.workspaceId}`);
  const picker = new SidebarFilterPopoverPage(testPage);
  const rename = await picker.beginNewView();
  await rename.fill("Workspace A view");
  await picker.popover.getByTestId("view-rename-confirm").click();
  await picker.close();
  await expect
    .poll(async () =>
      (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
        seedData.workspaceId
      ].views.some((view) => view.name === "Workspace A view"),
    )
    .toBe(true);

  await testPage.getByTestId("sidebar-workspace-trigger").click();
  await testPage.getByTestId(`sidebar-workspace-item-${other.id}`).click();
  await picker.expectActiveViewChip("All tasks");
  await picker.openViewPicker();
  await expect(picker.chipByName("Workspace A view")).toHaveCount(0);
  await picker.closeViewPicker();
  const renameB = await picker.beginNewView();
  await renameB.fill("Workspace B view");
  await picker.popover.getByTestId("view-rename-confirm").click();
  await picker.close();
  await expect
    .poll(async () =>
      (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[other.id].views.some(
        (view) => view.name === "Workspace B view",
      ),
    )
    .toBe(true);
  await testPage.reload();
  await picker.expectActiveViewChip("Workspace B view");

  await testPage.getByTestId("sidebar-workspace-trigger").click();
  await testPage.getByTestId(`sidebar-workspace-item-${seedData.workspaceId}`).click();
  await picker.expectActiveViewChip("Workspace A view");
  await picker.openViewPicker();
  await expect(picker.chipByName("Workspace B view")).toHaveCount(0);
  await testPage.screenshot({
    path: testInfo.outputPath("workspace-sidebar-desktop.png"),
    animations: "disabled",
  });
});
