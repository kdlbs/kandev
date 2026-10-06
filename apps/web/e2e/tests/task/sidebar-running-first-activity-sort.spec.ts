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
  sidebarRootOrder,
} from "./sidebar-running-first-activity-sort-helpers";

// @covers AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.1, .2, .3, .5, .6, .7, .10, .11, .13 AC-UI-SIDEBAR-GROUP-INDENT-001.1, .2, .3, .4, .6
test("desktop sorts a complete paged tree by running, color, and activity", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(180_000);
  const token = prCapture.capturing ? "Sidebar sort preview" : `Desktop sort ${Date.now()}`;
  const previousViews = await readPreviousSidebarViewState(apiClient, seedData.workspaceId);
  const red = await apiClient.createTask(seedData.workspaceId, `${token} zzz old red`, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const parent = await apiClient.createTask(seedData.workspaceId, `${token} old blue parent`, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const child = await apiClient.createTask(seedData.workspaceId, `${token} old red child`, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    parent_id: parent.id,
  });
  const { session_id: childSessionId } = await apiClient.seedTaskSession(child.id, {
    state: "RUNNING",
    agentProfileId: seedData.agentProfileId,
    startedAt: new Date(Date.now() - 60_000).toISOString(),
  });
  await apiClient.setPrimarySession(childSessionId);
  const otherIds: string[] = [];
  const taskOptions = {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  };
  for (let offset = 0; offset < 100; offset += 10) {
    const batch = await Promise.all(
      Array.from({ length: 10 }, (_, index) =>
        apiClient.seedTask(
          seedData.workspaceId,
          `${token} row ${String(offset + index).padStart(3, "0")}`,
          taskOptions,
        ),
      ),
    );
    otherIds.push(...batch.map((task) => task.task_id));
  }
  const navigation = await apiClient.createTask(seedData.workspaceId, "Desktop sort conversation", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const { session_id: conversationId } = await apiClient.seedTaskSession(navigation.id, {
    state: "COMPLETED",
    agentProfileId: seedData.agentProfileId,
    completedAt: new Date().toISOString(),
  });
  await apiClient.setPrimarySession(conversationId);
  const conversationText = "Sidebar sort keeps this conversation open";
  await apiClient.seedSessionMessage(conversationId, {
    type: "message",
    content: conversationText,
  });
  await apiClient.updateTaskState(navigation.id, "COMPLETED");
  const colorIds = [red.id, parent.id, child.id, ...otherIds];
  const colors: Record<string, "red" | "blue"> = {
    [red.id]: "red",
    [parent.id]: "blue",
    [child.id]: "red",
  };
  for (const id of otherIds) colors[id] = "blue";
  await apiClient.saveUserSettings({
    sidebar_task_color_patch: {
      colors,
      if_missing: false,
    },
  });
  const viewId = `desktop-sort-${Date.now()}`;
  await saveSidebarSortView(apiClient, seedData, viewId, token);
  const initialState = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
    seedData.workspaceId
  ];
  expect(initialState.active_view_id).toBe(viewId);
  expect(initialState.views.find((view) => view.id === viewId)?.sort).toMatchObject({
    key: "running",
    then_by: [{ key: "lastActivityAt", direction: "desc" }],
  });

  try {
    await testPage.goto(`/t/${navigation.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.activeChat().getByText(conversationText).last()).toBeVisible();
    const controls = session.sidebar.getByTestId("sidebar-page-controls");
    await expect(controls.getByText("Page 1 of 2")).toBeVisible({ timeout: 20_000 });
    await expect(session.sidebar.locator(`[data-task-row-id="${parent.id}"]`)).toBeVisible();
    await expect(session.sidebar.locator(`[data-task-row-id="${red.id}"]`)).toHaveCount(0);
    await expectSidebarRootOrder(session.sidebar, [parent.id, red.id], [parent.id]);

    await controls.getByRole("button").last().click();
    await expect(controls.getByText("Page 2 of 2")).toBeVisible();
    await expect(session.sidebar.locator(`[data-task-row-id="${red.id}"]`)).toBeVisible();

    const { filters, popover } = await openSidebarSortEditor(testPage, false);
    await filters.openSortSettings();
    await addColorAfterActivity(testPage, popover);
    if (prCapture.capturing) {
      await expect(popover.getByTestId("sort-rule-card-2")).toBeVisible();
      await waitForFiniteAnimations(popover);
      await prCapture.screenshot("sidebar-sort-chain-desktop", {
        caption: "Desktop sidebar sort chain: Running, Red, then newest activity",
      });
    }
    await expect(popover.getByRole("switch", { name: "Indent grouped tasks" })).toHaveCount(0);
    await filters.openGroupSettings();
    const indent = popover.getByRole("switch", { name: "Indent grouped tasks" });
    await expect(indent).toBeChecked();
    if (prCapture.capturing) {
      await waitForFiniteAnimations(popover);
      await prCapture.screenshot("sidebar-group-indent-desktop", {
        caption: "Desktop Group by settings with default grouped-task indentation enabled",
      });
    }
    const groupBody = session.sidebar
      .locator('[data-testid="sidebar-group"] [role="group"]')
      .first();
    await expect
      .poll(() => groupBody.evaluate((element) => element.classList.contains("ml-5")))
      .toBe(true);
    await filters.saveAs(`${token} chain`);
    await filters.close();

    await testPage.reload();
    await session.waitForLoad();
    await expect(session.activeChat().getByText(conversationText).last()).toBeVisible();
    const { filters: reloadFilters, popover: reloadPopover } = await openSidebarSortEditor(
      testPage,
      false,
    );
    await reloadFilters.openSortSettings();
    await expect(reloadPopover.getByTestId("sort-key-select")).toContainText("Running");
    await expect(reloadPopover.getByTestId("sort-rule-key-1")).toContainText("Color");
    await expect(reloadPopover.getByTestId("sort-rule-color-1")).toContainText("Red");
    await expect(reloadPopover.getByTestId("sort-rule-key-2")).toContainText("Last activity");
    await reloadFilters.close();

    await expect(controls.getByText("Page 1 of 2")).toBeVisible();
    await expectSidebarRootOrder(
      session.sidebar,
      [parent.id, red.id, ...otherIds],
      [parent.id, red.id],
    );
    await expect(session.sidebar.locator(`[data-task-row-id="${red.id}"]`)).toBeVisible();
    const parentTime = await session.sidebar
      .locator(`[data-task-row-id="${parent.id}"]`)
      .getByTestId("sidebar-task-time")
      .getAttribute("data-time-value");
    const childTime = await session.sidebar
      .locator(`[data-task-row-id="${child.id}"]`)
      .getByTestId("sidebar-task-time")
      .getAttribute("data-time-value");
    expect(parentTime).toBeTruthy();
    expect(childTime).toBeTruthy();
    expect(Date.parse(childTime!)).toBeGreaterThan(Date.parse(parentTime!));
    expect(
      await session.sidebar
        .locator(`[data-testid="sortable-task-block"][data-task-id="${child.id}"]`)
        .getAttribute("data-depth"),
    ).toBe("1");

    const beforeMove = await sidebarRootOrder(session.sidebar, [parent.id, red.id]);
    expect(beforeMove).toEqual([parent.id, red.id]);
    const { filters: precedenceFilters, popover: precedencePopover } = await openSidebarSortEditor(
      testPage,
      false,
    );
    await precedenceFilters.openSortSettings();
    await precedencePopover.getByTestId("sort-rule-up-1").click();
    await expectSidebarRootOrder(session.sidebar, [parent.id, red.id], [red.id, parent.id]);
    await precedencePopover.getByTestId("sort-rule-remove-0").click();
    await expectSidebarRootOrder(session.sidebar, [parent.id, red.id], [parent.id]);
    await expect(precedencePopover.getByTestId("sort-rule-key-1")).toContainText("Last activity");
    await precedenceFilters.close();
    await expect(controls.getByText("Page 1 of 2")).toBeVisible();
    await expect(testPage).toHaveURL(new RegExp(`/t/${navigation.id}$`));
    await expect(session.activeChat().getByText(conversationText).last()).toBeVisible();

    const { filters: restoreChainFilters, popover: restoreChainPopover } =
      await openSidebarSortEditor(testPage, false);
    await restoreChainFilters.openSortSettings();
    await addColorAfterActivity(testPage, restoreChainPopover);
    await restoreChainFilters.close();
    await expectSidebarRootOrder(session.sidebar, [parent.id, red.id], [parent.id, red.id]);
    await controls.getByRole("button").last().click();
    await expect(controls.getByText("Page 2 of 2")).toBeVisible();
    const { filters: groupFilters, popover: groupPopover } = await openSidebarSortEditor(
      testPage,
      false,
    );
    await groupFilters.openGroupSettings();
    const groupIndent = groupPopover.getByRole("switch", { name: "Indent grouped tasks" });
    await expect(groupIndent).toBeChecked();
    await groupIndent.click();
    await expect(groupIndent).not.toBeChecked();
    const groupDisclosure = groupPopover.getByTestId("sidebar-group-settings-toggle");
    await groupDisclosure.click();
    await expect(groupIndent).toHaveCount(0);
    await groupDisclosure.click();
    await expect(groupIndent).not.toBeChecked();
    await groupFilters.close();
    await expect(controls.getByText("Page 2 of 2")).toBeVisible();
    await groupFilters.open();
    await groupFilters.saveOverwrite();
    await groupFilters.close();
    await expect(testPage).toHaveURL(new RegExp(`/t/${navigation.id}$`));
    await expect(session.activeChat().getByText(conversationText).last()).toBeVisible();

    await testPage.reload();
    await session.waitForLoad();
    const { filters: falseIndentFilters, popover: falseIndentPopover } =
      await openSidebarSortEditor(testPage, false);
    await falseIndentFilters.openGroupSettings();
    const savedIndent = falseIndentPopover.getByRole("switch", { name: "Indent grouped tasks" });
    await expect(savedIndent).not.toBeChecked();
    const savedGroupBody = session.sidebar
      .locator('[data-testid="sidebar-group"] [role="group"]')
      .first();
    await expect
      .poll(() => savedGroupBody.evaluate((element) => element.classList.contains("ml-5")))
      .toBe(false);
    await falseIndentFilters.close();
    await expect(
      session.sidebar.locator(`[data-testid="sortable-task-block"][data-task-id="${child.id}"]`),
    ).toHaveAttribute("data-depth", "1");
  } finally {
    await cleanupSidebarSortColors(apiClient, colorIds);
    await restoreSidebarViewState(apiClient, seedData.workspaceId, previousViews);
  }
});
