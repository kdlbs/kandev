import { test, expect } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import { waitForSessionState } from "../../helpers/session";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { scrollSidebarFilterListTopIntoView, touchDragToPoint } from "../../helpers/touch-drag";
import { SessionPage } from "../../pages/session-page";
import {
  addColorAfterActivity,
  cleanupSidebarSortColors,
  expectSidebarRootOrder,
  openSidebarSortEditor,
  moveSortRuleWithMenu,
  readPreviousSidebarColorPatch,
  readPreviousSidebarViewState,
  readTaskRunningSummary,
  restoreSidebarSortColors,
  restoreSidebarViewState,
  saveSidebarRunningRankView,
  saveSidebarSortView,
  seedSidebarRunningRankScenario,
  waitForSidebarSortSync,
  waitForTaskRunningSummary,
  seedSidebarSortScenario,
  touchDragSortRule,
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

    const readSortKeys = () =>
      Promise.all([
        savedPopover.getByTestId("sort-key-select").innerText(),
        savedPopover.locator("[data-testid^='sort-rule-key-']").allTextContents(),
      ]).then(([primary, secondary]) => [primary, ...secondary]);
    const drawer = testPage.getByTestId("sidebar-filter-drawer");
    await savedPopover.evaluate((element) => element.scrollTo(0, 0));
    await waitForFiniteAnimations(savedPopover);
    const drawerBeforeDownwardDrag = await drawer.boundingBox();
    const firstHandle = savedPopover.getByTestId("sort-rule-handle-0");
    await expect(firstHandle).toHaveAttribute("data-vaul-no-drag", "");
    await touchDragSortRule(testPage, firstHandle, savedPopover.getByTestId("sort-rule-card-1"));
    await expect.poll(readSortKeys).toEqual(["Color", "Running", "Last activity"]);
    await expect(drawer).toBeVisible();
    await waitForFiniteAnimations(savedPopover);
    const drawerAfterDownwardDrag = await drawer.boundingBox();
    expect(drawerBeforeDownwardDrag).not.toBeNull();
    expect(drawerAfterDownwardDrag).not.toBeNull();
    expect(Math.abs(drawerAfterDownwardDrag!.y - drawerBeforeDownwardDrag!.y)).toBeLessThan(1);
    await moveSortRuleWithMenu(testPage, savedPopover, 2, "up");
    await expect.poll(readSortKeys).toEqual(["Running", "Color", "Last activity"]);
    await waitForSidebarSortSync(
      testPage,
      seedData.workspaceId,
      ["running", "color", "lastActivityAt"],
      "red",
    );
    const ruleList = savedPopover.locator("[data-sidebar-reorder-list]");
    await scrollSidebarFilterListTopIntoView(ruleList);
    await waitForFiniteAnimations(savedPopover);

    const lastHandle = savedPopover.getByTestId("sort-rule-handle-2");
    const scrollTop = await savedPopover.evaluate((element) => element.scrollTop);
    await savedPopover.evaluate((element) => {
      element.scrollTop += 24;
    });
    await waitForFiniteAnimations(ruleList);
    const ruleListBox = await ruleList.boundingBox();
    const editorBox = await savedPopover.boundingBox();
    expect(ruleListBox).not.toBeNull();
    expect(editorBox).not.toBeNull();
    const outsideY = editorBox!.y + 4;
    expect(outsideY).toBeGreaterThan(ruleListBox!.y);
    expect(outsideY).toBeLessThan(editorBox!.y + 12);
    await touchDragToPoint(
      testPage,
      lastHandle,
      {
        x: ruleListBox!.x + ruleListBox!.width / 2,
        y: outsideY,
      },
      async () => {
        await savedPopover.evaluate((element, previousScrollTop) => {
          element.scrollTop = previousScrollTop;
        }, scrollTop);
      },
    );
    await waitForFiniteAnimations(ruleList);
    await expect.poll(readSortKeys).toEqual(["Running", "Color", "Last activity"]);

    await moveSortRuleWithMenu(testPage, savedPopover, 3, "up");
    await expect.poll(readSortKeys).toEqual(["Running", "Last activity", "Color"]);
    await waitForSidebarSortSync(
      testPage,
      seedData.workspaceId,
      ["running", "lastActivityAt", "color"],
      "red",
    );
    await moveSortRuleWithMenu(testPage, savedPopover, 2, "up");
    await expect(savedPopover.getByTestId("sort-key-select")).toContainText("Last activity");
    await expect(savedPopover.getByTestId("sort-rule-key-1")).toContainText("Running");
    await waitForSidebarSortSync(
      testPage,
      seedData.workspaceId,
      ["lastActivityAt", "running", "color"],
      "red",
    );
    await moveSortRuleWithMenu(testPage, savedPopover, 1, "down");
    await expect(savedPopover.getByTestId("sort-key-select")).toContainText("Running");
    await waitForSidebarSortSync(
      testPage,
      seedData.workspaceId,
      ["running", "lastActivityAt", "color"],
      "red",
    );
    await moveSortRuleWithMenu(testPage, savedPopover, 3, "up");
    await expect(savedPopover.getByTestId("sort-rule-key-1")).toContainText("Color");
    await expect(savedPopover.getByTestId("sort-rule-key-2")).toContainText("Last activity");
    await waitForSidebarSortSync(
      testPage,
      seedData.workspaceId,
      ["running", "color", "lastActivityAt"],
      "red",
    );

    await moveSortRuleWithMenu(testPage, savedPopover, 2, "up");
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
    await expect(readdedColor).toBeFocused();
    await readdedColor.tap();
    await testPage.getByRole("option", { name: "Blue", exact: true }).tap();
    await expect(readdedColor).toBeFocused();
    await readdedColor.tap();
    await testPage.getByRole("option", { name: "Red", exact: true }).tap();
    await expect(readdedColor).toBeFocused();
    await moveSortRuleWithMenu(testPage, savedPopover, 3, "up");
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
    for (const action of ["sort-rule-handle-9", "sort-rule-more-9", "sort-rule-remove-9"]) {
      const box = await finalEditor.popover.getByTestId(action).boundingBox();
      expect(box?.height).toBeGreaterThanOrEqual(44);
      expect(box?.width).toBeGreaterThanOrEqual(44);
    }
    const lastCard = await lastRule.boundingBox();
    const controlBoxes = await Promise.all(
      ["sort-rule-handle-9", "sort-rule-more-9", "sort-rule-remove-9"].map((action) =>
        finalEditor.popover.getByTestId(action).boundingBox(),
      ),
    );
    for (const box of controlBoxes) {
      expect(box?.x).toBeGreaterThanOrEqual(lastCard!.x);
      expect(box!.x + box!.width).toBeLessThanOrEqual(lastCard!.x + lastCard!.width);
      expect(box?.y).toBeGreaterThanOrEqual(lastCard!.y);
      expect(box!.y + box!.height).toBeLessThanOrEqual(lastCard!.y + lastCard!.height);
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

// @covers AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.2, .6, .8, .14, .15, .16
test("phone task picker keeps task-wide running rank through secondary session changes", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  const token = prCapture.capturing ? "PhoneQ" : `phone-run-${Date.now()}`;
  const previousViews = await readPreviousSidebarViewState(apiClient, seedData.workspaceId);
  const previousColors = await readPreviousSidebarColorPatch(apiClient);
  const scenario = await seedSidebarRunningRankScenario(apiClient, seedData, token);
  const navigation = await apiClient.createTask(
    seedData.workspaceId,
    `Phone navigation conversation ${Date.now()}`,
    {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    },
  );
  const { session_id: conversationId } = await apiClient.seedTaskSession(navigation.id, {
    state: "COMPLETED",
    agentProfileId: seedData.agentProfileId,
    completedAt: new Date().toISOString(),
  });
  await apiClient.setPrimarySession(conversationId);
  const conversationText = "Phone task-wide sort keeps this conversation open";
  await apiClient.seedSessionMessage(conversationId, {
    type: "message",
    content: conversationText,
  });
  await apiClient.updateTaskState(navigation.id, "COMPLETED");
  const viewId = `phone-running-rank-${Date.now()}`;
  const watcher = watchWs(testPage);

  try {
    await apiClient.saveUserSettings({
      sidebar_task_color_patch: {
        colors: {
          [scenario.runningNoPrimary.id]: "blue",
          [scenario.secondaryTask.id]: "blue",
          [scenario.idleRed.id]: "red",
          [scenario.idleOrange.id]: "orange",
        },
        if_missing: false,
      },
    });
    await saveSidebarRunningRankView(apiClient, seedData, viewId, token);

    const noPrimarySessions = await apiClient.listTaskSessions(scenario.runningNoPrimary.id);
    expect(noPrimarySessions.sessions).toHaveLength(1);
    expect(noPrimarySessions.sessions[0]).toMatchObject({
      id: scenario.runningNoPrimarySessionId,
      state: "RUNNING",
      is_primary: false,
    });
    expect((await apiClient.getTask(scenario.runningNoPrimary.id)).primary_session_id ?? null).toBe(
      null,
    );
    const secondarySessions = await apiClient.listTaskSessions(scenario.secondaryTask.id);
    expect(secondarySessions.sessions).toHaveLength(2);
    expect(
      secondarySessions.sessions.find((session) => session.id === scenario.primarySessionId),
    ).toMatchObject({ state: "WAITING_FOR_INPUT", is_primary: true });
    expect(
      secondarySessions.sessions.find((session) => session.id === scenario.secondarySessionId),
    ).toMatchObject({ state: "WAITING_FOR_INPUT", is_primary: false });
    const savedView = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
      seedData.workspaceId
    ].views.find((view) => view.id === viewId);
    expect(savedView?.sort).toEqual({
      key: "running",
      direction: "desc",
      then_by: [
        { key: "color", color: "red", direction: "desc" },
        { key: "color", color: "orange", direction: "desc" },
        { key: "lastActivityAt", direction: "desc" },
      ],
    });

    await testPage.goto(`/t/${navigation.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const picker = testPage.getByTestId("mobile-task-picker-trigger");
    const sheet = testPage.getByRole("dialog", { name: "Tasks" });
    const expectConversationStable = async () => {
      await expect(testPage).toHaveURL(new RegExp(`/t/${navigation.id}$`));
      await expect(session.activeChat()).toHaveAttribute("data-session-id", conversationId);
      await expect(session.activeChat().getByText(conversationText).last()).toBeVisible();
    };
    const initialOrder = [
      scenario.runningNoPrimary.id,
      scenario.idleRed.id,
      scenario.idleOrange.id,
      scenario.secondaryTask.id,
    ];
    await expectConversationStable();
    await picker.tap();
    await expect(sheet).toBeVisible();
    await expectSidebarRootOrder(sheet, scenario.rootIds, initialOrder);
    await expect
      .poll(
        async () =>
          (
            await readTaskRunningSummary(
              apiClient,
              seedData.workspaceId,
              scenario.runningNoPrimary.id,
            )
          )?.has_running_session,
      )
      .toBe(true);
    await expect
      .poll(
        async () =>
          (await readTaskRunningSummary(apiClient, seedData.workspaceId, scenario.secondaryTask.id))
            ?.has_running_session,
      )
      .toBe(false);

    await testPage.reload();
    await session.waitForLoad();
    await expectConversationStable();
    await picker.tap();
    await expect(sheet).toBeVisible();
    await expectSidebarRootOrder(sheet, scenario.rootIds, initialOrder);

    const waitingSummary = await readTaskRunningSummary(
      apiClient,
      seedData.workspaceId,
      scenario.secondaryTask.id,
    );
    expect(waitingSummary?.has_running_session).toBe(false);
    if (!waitingSummary) throw new Error("Expected a task status summary before secondary start");
    const startedSummary = waitForTaskRunningSummary(
      watcher,
      scenario.secondaryTask.id,
      true,
      waitingSummary.revision,
    );
    await apiClient.seedTaskSession(scenario.secondaryTask.id, {
      state: "RUNNING",
      sessionId: scenario.secondarySessionId,
    });
    await waitForSessionState(apiClient, {
      taskId: scenario.secondaryTask.id,
      sessionId: scenario.secondarySessionId,
      expectedState: "RUNNING",
      message: "Waiting for the secondary session to start",
    });
    const startedEvent = await startedSummary;
    expect(startedEvent.payload.status_summary).toMatchObject({ has_running_session: true });
    await expect
      .poll(
        async () =>
          (await readTaskRunningSummary(apiClient, seedData.workspaceId, scenario.secondaryTask.id))
            ?.has_running_session,
      )
      .toBe(true);
    await expectSidebarRootOrder(sheet, scenario.rootIds, [
      scenario.secondaryTask.id,
      scenario.runningNoPrimary.id,
      scenario.idleRed.id,
      scenario.idleOrange.id,
    ]);
    await expectConversationStable();
    if (prCapture.capturing) {
      await waitForFiniteAnimations(sheet);
      await prCapture.screenshot("sidebar-running-first-rank-phone", {
        caption: "Phone Tasks drawer with the running secondary session ranked first",
      });
    }

    const runningSummary = await readTaskRunningSummary(
      apiClient,
      seedData.workspaceId,
      scenario.secondaryTask.id,
    );
    if (!runningSummary) throw new Error("Expected a task status summary before secondary stop");
    const stoppedSummary = waitForTaskRunningSummary(
      watcher,
      scenario.secondaryTask.id,
      false,
      runningSummary.revision,
    );
    await apiClient.seedTaskSession(scenario.secondaryTask.id, {
      state: "WAITING_FOR_INPUT",
      sessionId: scenario.secondarySessionId,
    });
    await waitForSessionState(apiClient, {
      taskId: scenario.secondaryTask.id,
      sessionId: scenario.secondarySessionId,
      expectedState: "WAITING_FOR_INPUT",
      message: "Waiting for the final running session to stop",
    });
    const stoppedEvent = await stoppedSummary;
    expect(stoppedEvent.payload.status_summary).toMatchObject({ has_running_session: false });
    await expect
      .poll(
        async () =>
          (await readTaskRunningSummary(apiClient, seedData.workspaceId, scenario.secondaryTask.id))
            ?.has_running_session,
      )
      .toBe(false);
    await expectSidebarRootOrder(sheet, scenario.rootIds, initialOrder);
    await expectConversationStable();
  } finally {
    try {
      await restoreSidebarSortColors(apiClient, scenario.colorIds, previousColors);
    } finally {
      await restoreSidebarViewState(apiClient, seedData.workspaceId, previousViews);
    }
  }
});
