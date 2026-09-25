import { test, expect } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { SessionPage } from "../../pages/session-page";
import { SidebarFilterPopoverPage } from "../../pages/sidebar-filter-popover";
import { measureFilterHeader } from "./sidebar-filter-spacing-helpers";

// @covers AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-001.7, AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-001.8
test("desktop filter heading and Add action sit below the View divider", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const navTask = await apiClient.seedTask(seedData.workspaceId, "Filter spacing desktop nav", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await testPage.goto(`/t/${navTask.task_id}`);
  await new SessionPage(testPage).waitForLoad();

  const filters = new SidebarFilterPopoverPage(testPage);
  await filters.open();
  const popover = filters.popover;
  await expect(popover).toBeVisible();
  await waitForFiniteAnimations(testPage.locator("body"));

  const geometry = await measureFilterHeader(popover);
  expect(geometry.labelTop - geometry.dividerBottom).toBeGreaterThanOrEqual(8);
  expect(geometry.addBox.y - geometry.dividerBottom).toBeGreaterThanOrEqual(8);
  expect(geometry.addBox.y).toBeGreaterThanOrEqual(geometry.sectionBox.y);
  expect(geometry.addBox.y + geometry.addBox.height).toBeLessThanOrEqual(
    geometry.sectionBox.y + geometry.sectionBox.height,
  );
  expect(geometry.addBox.x).toBeGreaterThanOrEqual(geometry.sectionBox.x);
  expect(geometry.addBox.x + geometry.addBox.width).toBeLessThanOrEqual(
    geometry.sectionBox.x + geometry.sectionBox.width,
  );
  await prCapture.screenshot("sidebar-filter-spacing-desktop", {
    caption: "Desktop Filters heading and Add action inset below the View divider.",
  });

  const sortBox = await popover.getByTestId("sidebar-sort-settings").boundingBox();
  expect(sortBox).not.toBeNull();
  expect(sortBox!.y).toBeGreaterThanOrEqual(geometry.sectionBox.y + geometry.sectionBox.height - 2);
  expect(sortBox!.y).toBeLessThanOrEqual(geometry.sectionBox.y + geometry.sectionBox.height + 4);

  for (let index = 0; index < 16; index += 1) {
    await popover.getByTestId("filter-add-button").click();
  }
  expect(await popover.evaluate((element) => element.scrollHeight > element.clientHeight)).toBe(
    true,
  );
  const automaticColors = popover.getByTestId("automatic-color-settings-toggle");
  await automaticColors.scrollIntoViewIfNeeded();
  await expect(automaticColors).toBeInViewport();
  const saveAs = popover.getByTestId("view-save-as-button");
  await saveAs.scrollIntoViewIfNeeded();
  await expect(saveAs).toBeInViewport();
});
