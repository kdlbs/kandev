import { test, expect } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { SessionPage } from "../../pages/session-page";
import { measureFilterHeader } from "./sidebar-filter-spacing-helpers";

// @covers AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-001.7, AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-001.8, AC-UI-CONTROL-SIZING-001.4
test("phone filter drawer keeps its inset, touch action, and single scroll owner", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const navTask = await apiClient.seedTask(seedData.workspaceId, "Filter spacing phone nav", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await testPage.goto(`/t/${navTask.task_id}`);
  await new SessionPage(testPage).waitForLoad();
  await testPage.getByTestId("mobile-task-picker-trigger").tap();

  const taskPicker = testPage.getByRole("dialog", { name: "Tasks" });
  await expect(taskPicker.getByTestId("sidebar-filter-bar")).toBeVisible();
  await taskPicker.getByTestId("sidebar-filter-gear").tap();

  const drawer = testPage.getByTestId("sidebar-filter-drawer");
  const popover = drawer.getByTestId("sidebar-filter-popover");
  await expect(drawer).toBeVisible();
  await expect(popover).toBeVisible();
  await waitForFiniteAnimations(testPage.locator("body"));

  const geometry = await measureFilterHeader(popover);
  expect(geometry.labelTop - geometry.dividerBottom).toBeGreaterThanOrEqual(8);
  expect(geometry.addBox.y - geometry.dividerBottom).toBeGreaterThanOrEqual(8);
  expect(geometry.addBox.height).toBeGreaterThanOrEqual(44);
  expect(geometry.addBox.y).toBeGreaterThanOrEqual(geometry.sectionBox.y);
  expect(geometry.addBox.y + geometry.addBox.height).toBeLessThanOrEqual(
    geometry.sectionBox.y + geometry.sectionBox.height,
  );
  await prCapture.screenshot("sidebar-filter-spacing-phone", {
    caption: "Phone Filters heading and 44px Add target in the scrolling drawer.",
  });
  await expect(popover).toHaveCSS("overflow-y", "auto");
  await expect(drawer).toHaveCSS("overflow-y", /^(hidden|clip)$/);

  for (let index = 0; index < 16; index += 1) {
    await popover.getByTestId("filter-add-button").tap();
  }
  expect(await popover.evaluate((element) => element.scrollHeight > element.clientHeight)).toBe(
    true,
  );
  const automaticColors = popover.getByTestId("automatic-color-settings-toggle");
  await automaticColors.scrollIntoViewIfNeeded();
  await expect(automaticColors).toBeInViewport();
  expect(
    await testPage.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
});
