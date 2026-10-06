import { test, expect } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { SessionPage } from "../../pages/session-page";
import {
  addColorAfterActivity,
  cleanupSidebarSortColors,
  expectSidebarRootOrder,
  openSidebarSortEditor,
  readPreviousSidebarViewState,
  restoreSidebarViewState,
  saveSidebarSortView,
  seedSidebarSortScenario,
} from "./sidebar-running-first-activity-sort-helpers";

// @covers AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.1, .2, .4, .5, .8, .11, .13 AC-UI-SIDEBAR-GROUP-INDENT-001.1, .2, .3, .4, .5, .6
test("phone drawer edits and saves a touch-reachable sort chain and group inset", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  const token = prCapture.capturing ? "Phone sidebar sort preview" : `Phone sort ${Date.now()}`;
  const previousViews = await readPreviousSidebarViewState(apiClient, seedData.workspaceId);
  const scenario = await seedSidebarSortScenario(apiClient, seedData, token);
  const navigation = await apiClient.createTask(seedData.workspaceId, "Phone sort conversation", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const { session_id: conversationId } = await apiClient.seedTaskSession(navigation.id, {
    state: "COMPLETED",
    agentProfileId: seedData.agentProfileId,
    completedAt: new Date().toISOString(),
  });
  await apiClient.setPrimarySession(conversationId);
  const conversationText = "Phone sort keeps this conversation open";
  await apiClient.seedSessionMessage(conversationId, {
    type: "message",
    content: conversationText,
  });
  await apiClient.updateTaskState(navigation.id, "COMPLETED");
  const viewId = `phone-sort-${Date.now()}`;
  let activeViewId = viewId;
  await saveSidebarSortView(apiClient, seedData, viewId, token);

  try {
    await testPage.goto(`/t/${navigation.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.activeChat().getByText(conversationText).last()).toBeVisible();
    const picker = testPage.getByTestId("mobile-task-picker-trigger");
    await picker.tap();
    const sheet = testPage.getByRole("dialog", { name: "Tasks" });
    await expectSidebarRootOrder(sheet, scenario.rootIds, [
      scenario.runningBlue.id,
      scenario.parent.id,
      scenario.idleBlue.id,
      scenario.idleRed.id,
    ]);

    const { filters, popover } = await openSidebarSortEditor(testPage, false);
    await filters.openSortSettings();
    await addColorAfterActivity(testPage, popover);
    if (prCapture.capturing) {
      await expect(popover.getByTestId("sort-rule-card-2")).toBeVisible();
      const firstRule = popover.getByTestId("sort-rule-card-0");
      await firstRule.scrollIntoViewIfNeeded();
      await expect(firstRule).toBeInViewport();
      await waitForFiniteAnimations(popover);
      await prCapture.screenshot("sidebar-sort-chain-phone", {
        caption: "Phone sidebar sort chain with stacked rule cards and move controls",
      });
    }
    await expect(popover.getByRole("switch", { name: "Indent grouped tasks" })).toHaveCount(0);
    await filters.openGroupSettings();
    const groupToggle = popover.getByRole("switch", { name: "Indent grouped tasks" });
    await expect(groupToggle).toBeChecked();
    if (prCapture.capturing) {
      await waitForFiniteAnimations(popover);
      await prCapture.screenshot("sidebar-group-indent-phone", {
        caption: "Phone Group by settings with grouped-task indentation enabled",
      });
    }
    const groupBody = sheet.locator('[data-testid="sidebar-group"] [role="group"]').first();
    await expect
      .poll(() => groupBody.evaluate((element) => element.classList.contains("ml-5")))
      .toBe(true);

    await filters.saveAs(`${token} chain`);
    await filters.close();
    const savedState = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
      seedData.workspaceId
    ];
    activeViewId = savedState.active_view_id;
    await expectSidebarRootOrder(sheet, scenario.rootIds, [
      scenario.parent.id,
      scenario.runningBlue.id,
      scenario.idleRed.id,
      scenario.idleBlue.id,
    ]);

    await testPage.reload();
    await session.waitForLoad();
    await expect(testPage).toHaveURL(new RegExp(`/t/${navigation.id}$`));
    await expect(session.activeChat().getByText(conversationText).last()).toBeVisible();
    await picker.tap();
    await expectSidebarRootOrder(sheet, scenario.rootIds, [
      scenario.parent.id,
      scenario.runningBlue.id,
      scenario.idleRed.id,
      scenario.idleBlue.id,
    ]);
    const { filters: savedFilters, popover: savedPopover } = await openSidebarSortEditor(
      testPage,
      false,
    );
    await savedFilters.openSortSettings();
    await expect(savedPopover.getByTestId("sort-key-select")).toContainText("Running");
    await expect(savedPopover.getByTestId("sort-rule-key-1")).toContainText("Color");
    await expect(savedPopover.getByTestId("sort-rule-color-1")).toContainText("Red");
    await expect(savedPopover.getByTestId("sort-rule-key-2")).toContainText("Last activity");

    await savedPopover.getByTestId("sort-rule-up-1").tap();
    await expectSidebarRootOrder(sheet, scenario.rootIds, [
      scenario.parent.id,
      scenario.idleRed.id,
      scenario.runningBlue.id,
      scenario.idleBlue.id,
    ]);
    await savedPopover.getByTestId("sort-rule-remove-0").tap();
    await expect(savedPopover.getByTestId("sort-key-select")).toContainText("Running");
    await expect(savedPopover.getByTestId("sort-rule-key-1")).toContainText("Last activity");
    await expectSidebarRootOrder(sheet, scenario.rootIds, [
      scenario.runningBlue.id,
      scenario.parent.id,
      scenario.idleBlue.id,
      scenario.idleRed.id,
    ]);
    await savedPopover.getByTestId("sort-add-rule-button").tap();
    await expect(savedPopover.getByTestId("sort-rule-card-2")).toBeVisible();
    await savedPopover.getByTestId("sort-rule-key-2").tap();
    await testPage.getByRole("option", { name: "Color", exact: true }).tap();
    const readdedColor = savedPopover.getByTestId("sort-rule-color-2");
    await expect(savedPopover.getByTestId("sort-rule-key-2")).toContainText("Color");
    await expect(readdedColor).toBeVisible();
    await readdedColor.scrollIntoViewIfNeeded();
    await readdedColor.tap();
    await testPage.getByRole("option", { name: "Red", exact: true }).tap();
    await savedPopover.getByTestId("sort-rule-up-2").tap();
    await expectSidebarRootOrder(sheet, scenario.rootIds, [
      scenario.parent.id,
      scenario.runningBlue.id,
      scenario.idleRed.id,
      scenario.idleBlue.id,
    ]);

    await savedFilters.openGroupSettings();
    await expect(groupToggle).toBeVisible();
    await expect(groupToggle).toBeChecked();
    const pageControlCountBefore = await sheet.getByTestId("sidebar-page-controls").count();
    await groupToggle.tap();
    await expect(groupToggle).not.toBeChecked();
    await expect
      .poll(() => groupBody.evaluate((element) => element.classList.contains("ml-5")))
      .toBe(false);
    const beforeDepth = await sheet
      .locator(`[data-testid="sortable-task-block"][data-task-id="${scenario.child.id}"]`)
      .getAttribute("data-depth");
    expect(beforeDepth).toBe("1");
    const groupDisclosure = savedPopover.getByTestId("sidebar-group-settings-toggle");
    await groupDisclosure.click();
    await expect(groupToggle).toHaveCount(0);
    await groupDisclosure.click();
    await expect(groupToggle).not.toBeChecked();
    await expect
      .poll(() => groupBody.evaluate((element) => element.classList.contains("ml-5")))
      .toBe(false);
    expect(await sheet.getByTestId("sidebar-page-controls").count()).toBe(pageControlCountBefore);
    await savedFilters.saveOverwrite();
    await savedFilters.close();

    const { settings } = await apiClient.getUserSettings();
    const currentState = settings.sidebar_views_by_workspace[seedData.workspaceId];
    const savedView = currentState.views.find((view) => view.id === activeViewId);
    expect(savedView?.group_indent).toBe(false);
    expect(savedView?.sort).toMatchObject({
      key: "running",
      then_by: [
        { key: "color", color: "red", direction: "desc" },
        { key: "lastActivityAt", direction: "desc" },
      ],
    });

    await testPage.reload();
    await session.waitForLoad();
    await picker.tap();
    const { filters: reloadedFilters, popover: reloadedPopover } = await openSidebarSortEditor(
      testPage,
      false,
    );
    await reloadedFilters.openGroupSettings();
    const reloadedToggle = reloadedPopover.getByRole("switch", { name: "Indent grouped tasks" });
    await expect(reloadedToggle).not.toBeChecked();
    await reloadedFilters.openSortSettings();
    await expect(reloadedPopover.getByTestId("sort-rule-key-1")).toContainText("Color");
    await expect(reloadedPopover.getByTestId("sort-rule-color-1")).toContainText("Red");
    await reloadedFilters.close();

    const finalEditor = await openSidebarSortEditor(testPage, false);
    await finalEditor.filters.openSortSettings();
    for (let index = 3; index < 10; index += 1) {
      await finalEditor.popover.getByTestId("sort-add-rule-button").tap();
    }
    await expect(finalEditor.popover.getByTestId("sort-rule-card-9")).toBeAttached();
    await expect(finalEditor.popover.getByTestId("sort-add-rule-button")).toBeDisabled();
    expect(
      await finalEditor.popover.evaluate((element) => element.scrollHeight > element.clientHeight),
    ).toBe(true);
    const lastRule = finalEditor.popover.getByTestId("sort-rule-card-9");
    await lastRule.scrollIntoViewIfNeeded();
    await expect(lastRule).toBeInViewport();
    for (const action of ["sort-rule-up-9", "sort-rule-down-9", "sort-rule-remove-9"]) {
      const box = await finalEditor.popover.getByTestId(action).boundingBox();
      expect(box?.height).toBeGreaterThanOrEqual(44);
    }
    const viewport = await testPage.evaluate(() => ({ width: innerWidth, height: innerHeight }));
    const drawerBox = await testPage.getByTestId("sidebar-filter-drawer").boundingBox();
    expect(drawerBox?.x).toBeGreaterThanOrEqual(0);
    expect(drawerBox!.x + drawerBox!.width).toBeLessThanOrEqual(viewport.width);
    expect(drawerBox?.y).toBeGreaterThanOrEqual(0);
    expect(drawerBox!.y + drawerBox!.height).toBeLessThanOrEqual(viewport.height);
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await expect(testPage).toHaveURL(new RegExp(`/t/${navigation.id}$`));
    await expect(session.activeChat().getByText(conversationText).last()).toBeVisible();

    await finalEditor.filters.close();
    await sheet.locator(`[data-task-row-id="${scenario.child.id}"]`).tap();
    await expect(testPage).toHaveURL(new RegExp(`/t/${scenario.child.id}$`));
  } finally {
    await cleanupSidebarSortColors(apiClient, scenario.colorIds);
    await restoreSidebarViewState(apiClient, seedData.workspaceId, previousViews);
  }
});
