import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { SidebarFilterPopoverPage } from "../../pages/sidebar-filter-popover";

function taskRow(sidebar: import("@playwright/test").Locator, taskId: string) {
  return sidebar.locator(`[data-task-row-id="${taskId}"]`);
}

async function readRowLayout(row: import("@playwright/test").Locator, taskId: string) {
  const title = row.locator("span.overflow-hidden").first();
  const prIcon = row.getByTestId(`pr-task-icon-${taskId}`);
  const relativeTime = row.getByTestId("sidebar-task-trailing-time");
  const timeSlot = relativeTime.locator("xpath=..");
  const action = row.locator('button[aria-label="Task actions"]');
  const visualTime = relativeTime.locator('[aria-hidden="true"]');
  const [titleBox, prBox, slotBox, actionBox, visualTimeBox] = await Promise.all([
    title.boundingBox(),
    prIcon.boundingBox(),
    timeSlot.boundingBox(),
    action.boundingBox(),
    visualTime.boundingBox(),
  ]);

  expect(titleBox).not.toBeNull();
  expect(prBox).not.toBeNull();
  expect(slotBox).not.toBeNull();
  expect(actionBox).not.toBeNull();
  expect(visualTimeBox).not.toBeNull();
  return {
    title: titleBox!,
    pr: prBox!,
    slot: slotBox!,
    action: actionBox!,
    visualTime: visualTimeBox!,
  };
}

async function expectContentSizedTimeSlot(row: import("@playwright/test").Locator) {
  const relativeTime = row.getByTestId("sidebar-task-trailing-time");
  const timeSlot = relativeTime.locator("xpath=..");
  const visualTime = relativeTime.locator('[aria-hidden="true"]');
  const action = row.locator("button.mobile-task-actions-button");
  const [slotBox, visualTimeBox, actionBox] = await Promise.all([
    timeSlot.boundingBox(),
    visualTime.boundingBox(),
    action.boundingBox(),
  ]);

  expect(slotBox).not.toBeNull();
  expect(visualTimeBox).not.toBeNull();
  expect(actionBox).not.toBeNull();
  expect(slotBox!.width).toBeCloseTo(Math.max(visualTimeBox!.width, actionBox!.width), 1);
}

function expectStableLayout(
  baseline: Awaited<ReturnType<typeof readRowLayout>>,
  current: Awaited<ReturnType<typeof readRowLayout>>,
) {
  for (const key of ["x", "y", "width", "height"] as const) {
    expect(current.title[key]).toBeCloseTo(baseline.title[key], 1);
    expect(current.pr[key]).toBeCloseTo(baseline.pr[key], 1);
    expect(current.slot[key]).toBeCloseTo(baseline.slot[key], 1);
  }
}

test("sidebar title width follows compact time without hover movement", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const longTitle = "Keep the linked pull request beside this long task title";
  const shortTitle = "Short sidebar title";
  const longTask = await apiClient.createTask(seedData.workspaceId, longTitle, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  const shortTask = await apiClient.createTask(seedData.workspaceId, shortTitle, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  const shortTaskWithoutPR = await apiClient.createTask(
    seedData.workspaceId,
    "Short title without a pull request",
    {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  const navigationTask = await apiClient.createTask(seedData.workspaceId, "Title width nav", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });

  for (const task of [longTask, shortTask]) {
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      workspace_id: seedData.workspaceId,
      repository_id: seedData.repositoryId,
      owner: "testorg",
      repo: "testrepo",
      pr_number: task.id === longTask.id ? 941 : 942,
      pr_url: `https://github.com/testorg/testrepo/pull/${task.id === longTask.id ? 941 : 942}`,
      pr_title: "Sidebar title width",
      head_branch: "feature/sidebar-title-width",
      base_branch: "main",
      author_login: "e2e",
      state: "open",
      checks_state: "success",
    });
  }

  await testPage.clock.setFixedTime(
    new Date(Date.parse(longTask.updated_at) + 2 * 60 * 60 * 1000 + 5 * 60 * 1000),
  );
  await testPage.goto(`/t/${navigationTask.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.sidebar).toBeVisible();
  expect(
    await testPage.evaluate(() => matchMedia("(min-width: 640px) and (pointer: fine)").matches),
  ).toBe(true);

  const settingsBefore = await apiClient.getUserSettings();
  const previousViewState =
    settingsBefore.settings.sidebar_views_by_workspace[seedData.workspaceId];
  const filters = new SidebarFilterPopoverPage(testPage);

  try {
    await testPage.setViewportSize({ width: 1280, height: 900 });
    await filters.open();
    await filters.openTaskRowSettings();
    await filters.setTaskRowTrailing("Relative time");
    await filters.saveOverwrite();
    await filters.close();

    const longRow = taskRow(session.sidebar, longTask.id);
    const shortRow = taskRow(session.sidebar, shortTask.id);
    const noPRRow = taskRow(session.sidebar, shortTaskWithoutPR.id);
    await expect(longRow.getByTestId("sidebar-task-trailing-time")).toBeVisible();
    await expect(shortRow.getByTestId(`pr-task-icon-${shortTask.id}`)).toBeVisible({
      timeout: 15_000,
    });
    await expect(
      noPRRow.getByText("Short title without a pull request", { exact: true }),
    ).toBeVisible();

    const baseline = await readRowLayout(longRow, longTask.id);
    await expect(
      longRow.getByTestId("sidebar-task-trailing-time").locator('[aria-hidden="true"]'),
    ).toHaveText("2h");
    expect(baseline.visualTime.width).toBeLessThan(24);
    expect(
      Math.abs(
        baseline.visualTime.x + baseline.visualTime.width - (baseline.slot.x + baseline.slot.width),
      ),
    ).toBeLessThanOrEqual(1);
    expect(baseline.pr.x - (baseline.title.x + baseline.title.width)).toBeCloseTo(4, 1);
    expect(baseline.slot.width).toBeCloseTo(
      Math.max(baseline.visualTime.width, baseline.action.width),
      1,
    );

    const shortLayout = await readRowLayout(shortRow, shortTask.id);
    expect(shortLayout.pr.x - (shortLayout.title.x + shortLayout.title.width)).toBeCloseTo(4, 1);

    const noPRTitle = noPRRow.locator("span.overflow-hidden").first();
    const noPRTime = noPRRow.getByTestId("sidebar-task-trailing-time");
    const noPRSlot = noPRTime.locator("xpath=..");
    const noPRAction = noPRRow.locator('button[aria-label="Task actions"]');
    const noPRVisualTime = noPRTime.locator('[aria-hidden="true"]');
    const [noPRTitleBox, noPRSlotBox, noPRActionBox, noPRTimeBox] = await Promise.all([
      noPRTitle.boundingBox(),
      noPRSlot.boundingBox(),
      noPRAction.boundingBox(),
      noPRVisualTime.boundingBox(),
    ]);
    expect(noPRTitleBox).not.toBeNull();
    expect(noPRSlotBox).not.toBeNull();
    expect(noPRActionBox).not.toBeNull();
    expect(noPRTimeBox).not.toBeNull();
    expect(await noPRRow.getByTestId(`pr-task-icon-${shortTaskWithoutPR.id}`).count()).toBe(0);
    expect(noPRSlotBox!.width).toBeCloseTo(Math.max(noPRTimeBox!.width, noPRActionBox!.width), 1);

    const actions = longRow.locator('button[aria-label="Task actions"]');
    const actionWrapper = actions.locator("xpath=..");
    await longRow.hover();
    await expect(actionWrapper).toHaveCSS("opacity", "1");
    expectStableLayout(baseline, await readRowLayout(longRow, longTask.id));

    await testPage.mouse.move(0, 0);
    await longRow.focus();
    expectStableLayout(baseline, await readRowLayout(longRow, longTask.id));

    await actions.focus();
    await expect(actionWrapper).toHaveCSS("opacity", "1");
    expectStableLayout(baseline, await readRowLayout(longRow, longTask.id));

    await longRow.click({ button: "right" });
    const contextMenu = testPage.getByRole("menu");
    await expect(contextMenu).toBeVisible();
    expectStableLayout(baseline, await readRowLayout(longRow, longTask.id));
    await testPage.keyboard.press("Escape");
    await expect(contextMenu).toBeHidden();
    expectStableLayout(baseline, await readRowLayout(longRow, longTask.id));

    await filters.open();
    await filters.openTaskRowSettings();
    await filters.taskRowSettings.getByTestId("task-row-details-toggle").click();
    await filters.saveOverwrite();
    await filters.close();
    const compactDetailsLayout = await readRowLayout(longRow, longTask.id);
    expect(compactDetailsLayout.title.width).toBeCloseTo(baseline.title.width, 1);
    expect(
      compactDetailsLayout.pr.x - (compactDetailsLayout.title.x + compactDetailsLayout.title.width),
    ).toBeCloseTo(4, 1);
    expect(compactDetailsLayout.slot.width).toBeCloseTo(
      Math.max(compactDetailsLayout.visualTime.width, compactDetailsLayout.action.width),
      1,
    );

    const sidebarLayout = testPage.getByTestId("app-sidebar-layout");
    const narrowSidebarBox = await sidebarLayout.boundingBox();
    const resizeHandle = testPage.getByRole("button", { name: /resize sidebar/i });
    const resizeHandleBox = await resizeHandle.boundingBox();
    expect(narrowSidebarBox).not.toBeNull();
    expect(resizeHandleBox).not.toBeNull();
    await testPage.mouse.move(
      resizeHandleBox!.x + resizeHandleBox!.width / 2,
      resizeHandleBox!.y + 120,
    );
    await testPage.mouse.down();
    await testPage.mouse.move(
      resizeHandleBox!.x + resizeHandleBox!.width / 2 + 60,
      resizeHandleBox!.y + 120,
      { steps: 8 },
    );
    await testPage.mouse.up();
    await expect
      .poll(async () => (await sidebarLayout.boundingBox())?.width ?? 0)
      .toBeGreaterThan(narrowSidebarBox!.width + 40);
    const expandedLayout = await readRowLayout(longRow, longTask.id);
    expect(expandedLayout.title.width).toBeGreaterThan(compactDetailsLayout.title.width + 40);
    expect(expandedLayout.slot.width).toBeCloseTo(
      Math.max(expandedLayout.visualTime.width, expandedLayout.action.width),
      1,
    );

    await prCapture.screenshot("desktop-sidebar-title-width", {
      caption: "Desktop task title and PR icon keep their geometry beside the compact time slot",
    });
  } finally {
    const response = await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      sidebar_view_state: {
        workspace_id: seedData.workspaceId,
        views: previousViewState.views,
        active_view_id: previousViewState.active_view_id,
        draft: previousViewState.draft,
      },
    });
    expect(response.ok).toBe(true);
  }
});

test("localized time slots respect the fine-pointer width breakpoint", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.createTask(seedData.workspaceId, "修复侧边栏任务标题", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  const navigationTask = await apiClient.createTask(seedData.workspaceId, "Localized width nav", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });

  await testPage.setViewportSize({ width: 1280, height: 900 });
  await testPage.goto(`/t/${navigationTask.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  const filters = new SidebarFilterPopoverPage(testPage);
  await filters.open();
  await filters.openTaskRowSettings();
  await filters.setTaskRowTrailing("Relative time");
  await filters.saveOverwrite();
  await filters.close();

  for (const locale of ["zh-cn", "pseudo"]) {
    const origin = new URL(testPage.url()).origin;
    await testPage.context().addCookies([{ name: "kandev_locale", value: locale, url: origin }]);
    await testPage.reload();
    await session.waitForLoad();
    await expect(testPage.locator("html")).toHaveAttribute("lang", locale);
    const row = taskRow(session.sidebar, task.id);
    await expect(row.getByTestId("sidebar-task-trailing-time")).toBeVisible();
    await expectContentSizedTimeSlot(row);
  }

  for (const width of [768, 640, 639]) {
    await testPage.setViewportSize({ width, height: 900 });
    expect(await testPage.evaluate(() => matchMedia("(pointer: fine)").matches)).toBe(true);

    if (width === 640) {
      await testPage.getByTestId("mobile-task-picker-trigger").click();
    }

    const row = testPage.locator(
      `[data-testid="sidebar-task-item"][data-task-row-id="${task.id}"]:visible`,
    );
    await expect(row).toHaveCount(1);
    await expect(row.getByTestId("sidebar-task-trailing-time")).toBeVisible();

    if (width === 639) {
      const relativeTime = row.getByTestId("sidebar-task-trailing-time");
      const action = row.locator("button.mobile-task-actions-button");
      const [rowBox, timeBox, actionBox] = await Promise.all([
        row.boundingBox(),
        relativeTime.boundingBox(),
        action.boundingBox(),
      ]);
      expect(rowBox).not.toBeNull();
      expect(timeBox).not.toBeNull();
      expect(actionBox).not.toBeNull();
      expect(timeBox!.width).toBeGreaterThanOrEqual(44);
      expect(actionBox!.width).toBeGreaterThanOrEqual(44);
      expect(actionBox!.height).toBeGreaterThanOrEqual(44);
      expect(timeBox!.x + timeBox!.width).toBeLessThanOrEqual(actionBox!.x + 1);
      expect(actionBox!.x).toBeGreaterThanOrEqual(rowBox!.x - 1);
      expect(actionBox!.x + actionBox!.width).toBeLessThanOrEqual(rowBox!.x + rowBox!.width + 1);
      expect(
        await testPage.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      ).toBe(true);
    } else {
      await expectContentSizedTimeSlot(row);
    }
  }
});
