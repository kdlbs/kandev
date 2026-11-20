import { expect, test } from "../../fixtures/test-base";
import {
  installFixturePlugin,
  uninstallFixturePlugin,
  PLUGIN_ID,
} from "../../helpers/plugin-fixture";
import {
  SIDEBAR_GROUPING,
  groupingValueKey,
  seedCompleteGrouping,
  saveGroupingView,
  writeGroupingState,
} from "./sidebar-plugin-grouping-helpers";
import { exerciseChangedGroupingDraft } from "./sidebar-grouping-draft-helpers";
import { holdSidebarSettingsWrite } from "../../helpers/user-settings-barrier";

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.1
// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.6
test.afterEach(async ({ apiClient }) => {
  await uninstallFixturePlugin(apiClient);
});

test("phone task views create and restore an ordinary plugin grouping with reachable touch controls", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  test.setTimeout(120_000);
  await installFixturePlugin(testPage);
  await testPage.goto("/");
  await testPage.setViewportSize({ width: 320, height: 700 });
  await testPage.addStyleTag({ content: "html { font-size: 12px; }" });
  await testPage.getByTestId("app-nav-trigger").tap();
  const tasks = testPage.getByTestId("app-nav-sheet");
  await expect(tasks).toBeVisible();
  const trigger = tasks.getByRole("button", { name: "Plugin groupings", exact: true });
  await expect(trigger).toBeVisible();
  const box = await trigger.boundingBox();
  expect(box!.height).toBeGreaterThanOrEqual(44);
  await trigger.tap();
  const chooser = testPage.getByRole("dialog", { name: "Plugin groupings", exact: true });
  const choice = chooser.getByRole("button", { name: "Fixture grouping", exact: true });
  await expect(choice).toBeVisible();
  const choiceBox = await choice.boundingBox();
  expect(choiceBox!.height).toBeGreaterThanOrEqual(44);
  expect(choiceBox!.x).toBeGreaterThanOrEqual(0);
  expect(choiceBox!.x + choiceBox!.width).toBeLessThanOrEqual(testPage.viewportSize()!.width);
  const choices = chooser.getByTestId("sidebar-grouping-choices");
  await expect
    .poll(() => choices.evaluate((element) => element.scrollHeight > element.clientHeight))
    .toBe(true);
  const wrapped = choices.getByTestId("sidebar-plugin-grouping").last();
  await wrapped.scrollIntoViewIfNeeded();
  expect(await choices.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
  const wrappedBox = await wrapped.boundingBox();
  const labelBox = await wrapped.locator("span").first().boundingBox();
  expect(labelBox!.y).toBeGreaterThanOrEqual(wrappedBox!.y);
  expect(labelBox!.y + labelBox!.height).toBeLessThanOrEqual(wrappedBox!.y + wrappedBox!.height);
  expect(wrappedBox!.y).toBeGreaterThanOrEqual(0);
  expect(wrappedBox!.y + wrappedBox!.height).toBeLessThanOrEqual(testPage.viewportSize()!.height);
  expect(
    await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
  ).toBe(true);
  await testPage.screenshot({
    path: testInfo.outputPath("phone-grouping-chooser.png"),
    animations: "disabled",
  });
  await testPage.keyboard.press("Escape");
  await expect(chooser).toBeHidden();
  await expect(trigger).toBeFocused();
  const selection = await holdSidebarSettingsWrite(testPage);
  try {
    await trigger.tap();
    await chooser.getByRole("button", { name: "Fixture grouping", exact: true }).tap();
    await selection.arrived;
    await expect(chooser).toBeHidden();
    await expect(trigger).toHaveAttribute("aria-busy", "true");
    await expect(trigger).toHaveAttribute("aria-disabled", "true");
    const all = tasks.getByTestId("sidebar-view-chip").filter({ hasText: "All tasks" });
    await expect(all).toBeDisabled();
    const allBox = await all.boundingBox();
    await testPage.touchscreen.tap(allBox!.x + allBox!.width / 2, allBox!.y + allBox!.height / 2);
    await expect(tasks.getByTestId("sidebar-filter-gear")).toHaveAttribute("aria-disabled", "true");
    expect(selection.writes).toBe(1);
    await testPage.screenshot({
      path: testInfo.outputPath("phone-pending-grouping.png"),
      animations: "disabled",
    });
  } finally {
    selection.release();
    await selection.dispose();
  }
  await expect(
    tasks.getByTestId("sidebar-view-chip").filter({ hasText: "Fixture grouping" }),
  ).toHaveAttribute("data-active", "true");
  await expect
    .poll(async () => {
      const { settings } = await apiClient.getUserSettings();
      const state = settings.sidebar_views_by_workspace[seedData.workspaceId];
      return state.views.find((view) => view.id === state.active_view_id)?.group;
    })
    .toBe("plugin:kandev-plugin-e2e:fixture-labels");
  expect(
    await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
  ).toBe(true);
  await testPage.screenshot({
    path: testInfo.outputPath("phone-grouped-task-views.png"),
    animations: "disabled",
  });
  const saved = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
    seedData.workspaceId
  ];
  await testPage.reload();
  await testPage.getByTestId("app-nav-trigger").tap();
  await expect(
    tasks.getByTestId("sidebar-view-chip").filter({ hasText: "Fixture grouping" }),
  ).toHaveAttribute("data-active", "true");
  await trigger.tap();
  await chooser.getByRole("button", { name: "Fixture grouping", exact: true }).tap();
  await expect
    .poll(async () => {
      const state = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
        seedData.workspaceId
      ];
      return [state.views.length, state.active_view_id];
    })
    .toEqual([saved.views.length, saved.active_view_id]);
});

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.2
// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.3
// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.4
// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.6
test("phone complete grouped pages survive live changes and workspace switching", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  test.setTimeout(180_000);
  await installFixturePlugin(testPage);
  const other = await apiClient.createWorkspace("Other grouping workspace");
  await apiClient.createWorkflow(other.id, "Other grouping workflow", "simple");
  const data = await seedCompleteGrouping(apiClient, seedData);
  await saveGroupingView(apiClient, seedData.workspaceId, data.roots[90].id);
  await testPage.goto(`/?workspaceId=${seedData.workspaceId}`);
  await testPage.getByTestId("app-nav-trigger").tap();
  const tasks = testPage.getByTestId("app-nav-sheet");
  const alpha = tasks.locator(
    `[data-testid="sidebar-group"][data-group-key="${groupingValueKey("late")}"]`,
  );
  const controls = tasks.getByTestId("sidebar-page-controls");
  await expect(alpha.getByTestId("sidebar-task-item")).toHaveCount(3);
  await expect(alpha).toContainText(data.roots[100].title);
  await expect(controls).toContainText("103 unique tasks. Page 1 of 2 (occurrences).");
  const next = controls.getByRole("button", { name: "Next", exact: true });
  expect((await next.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await next.scrollIntoViewIfNeeded();
  await expect(next).toBeInViewport({ ratio: 1 });
  await testPage.screenshot({
    path: testInfo.outputPath("complete-grouping-phone-pager-before-tap.png"),
    animations: "disabled",
  });
  await next.tap();
  await expect(controls).toContainText("Page 2 of 2");
  await expect(tasks).toContainText(data.untagged.title);
  await testPage.screenshot({
    path: testInfo.outputPath("complete-grouping-phone-page-two.png"),
    animations: "disabled",
  });
  const updated = {
    ...data.snapshot,
    revision: "phone-r2",
    groups: data.snapshot.groups.map((group) =>
      group.id === "late" ? { ...group, label: "Phone live group", color: "#aa44dd" } : group,
    ),
  };
  await writeGroupingState(apiClient, seedData.workspaceId, { snapshot: updated });
  await expect(alpha.getByTestId("sidebar-group-header")).toContainText("Phone live group");
  await expect(controls).toContainText("Page 1 of 2");
  await alpha.getByTestId("sidebar-group-header").tap();
  await expect(alpha.getByTestId("sidebar-group-header")).toHaveAttribute("aria-expanded", "false");
  await expect
    .poll(async () => {
      const state = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
        seedData.workspaceId
      ];
      return state.views.find((view) => view.id === state.active_view_id)?.collapsed_groups;
    })
    .toEqual([groupingValueKey("late")]);
  await testPage.getByTestId("mobile-workspace-trigger").tap();
  await testPage.getByTestId(`mobile-workspace-item-${other.id}`).tap();
  await testPage.getByTestId("app-nav-trigger").tap();
  await expect(tasks.getByTestId("sidebar-view-chip")).toHaveCount(1);
  await expect(tasks.getByTestId("sidebar-view-chip")).toContainText("All tasks");
  await expect(tasks.getByTestId("sidebar-group")).toHaveCount(0);
  await testPage.getByTestId("mobile-workspace-trigger").tap();
  await testPage.getByTestId(`mobile-workspace-item-${seedData.workspaceId}`).tap();
  await testPage.getByTestId("app-nav-trigger").tap();
  await expect(
    tasks.getByTestId("sidebar-view-chip").filter({ hasText: "Complete view" }),
  ).toHaveAttribute("data-active", "true");
  await expect(alpha.getByTestId("sidebar-group-header")).toHaveAttribute("aria-expanded", "false");
  await expect(alpha.getByTestId("sidebar-group-header")).toContainText("Phone live group");
  expect(
    (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[other.id].views.map(
      (view) => view.group,
    ),
  ).not.toContain(SIDEBAR_GROUPING);
  expect(
    await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
  ).toBe(true);
  await testPage.screenshot({
    path: testInfo.outputPath("complete-grouping-phone-restored.png"),
    animations: "disabled",
  });
  await testPage.keyboard.press("Escape");
  await testPage.setViewportSize({ width: 1200, height: 900 });
  await expect(testPage.getByTestId("task-sidebar")).toBeVisible();
  const coarseNext = testPage
    .getByTestId("task-sidebar")
    .getByRole("button", { name: "Next", exact: true });
  expect(await testPage.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(true);
  expect((await coarseNext.boundingBox())!.height).toBeGreaterThanOrEqual(44);
});

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.1
// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.3
// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.6
test("phone task switcher restores parent touch geometry after choosing a grouping", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  test.setTimeout(180_000);
  await installFixturePlugin(testPage);
  const data = await seedCompleteGrouping(apiClient, seedData);
  await saveGroupingView(apiClient, seedData.workspaceId, data.roots[90].id);
  await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: true });
  await testPage.goto(`/t/${data.roots[100].id}`);
  await testPage.getByTestId("mobile-task-picker-trigger").tap();
  const tasks = testPage.getByRole("dialog", { name: "Tasks", exact: true });
  await expect(tasks.getByTestId("sidebar-page-controls")).toContainText("103 unique tasks");
  await tasks.getByRole("button", { name: "Plugin groupings", exact: true }).tap();
  const chooser = testPage.getByRole("dialog", { name: "Plugin groupings", exact: true });
  await chooser.getByRole("button", { name: "Fixture grouping", exact: true }).tap();
  await expect(chooser).toBeHidden();
  await expect(tasks).toBeVisible();
  const controls = tasks.getByTestId("sidebar-page-controls");
  const next = controls.getByRole("button", { name: "Next", exact: true });
  await expect(next).toBeEnabled();
  await expect.poll(async () => (await next.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
  await expect.poll(async () => (await next.boundingBox())?.width ?? 0).toBeGreaterThanOrEqual(44);
  await next.scrollIntoViewIfNeeded();
  await expect(next).toBeInViewport({ ratio: 1 });
  await testPage.screenshot({
    path: testInfo.outputPath("task-switcher-grouping-parent-restored.png"),
    animations: "disabled",
  });
  await next.tap();
  await expect(controls).toContainText("Page 2 of 2");
  await expect(tasks).toContainText(data.untagged.title);
  await expect(testPage).toHaveURL((url) => url.pathname === `/t/${data.roots[100].id}`);
  expect(
    await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
  ).toBe(true);
  await testPage.screenshot({
    path: testInfo.outputPath("task-switcher-grouping-page-two.png"),
    animations: "disabled",
  });
});

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.4
// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.5
// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.6
test("phone task switcher restores its controls when an open grouping provider disappears", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await installFixturePlugin(testPage);
  const task = await apiClient.createTask(seedData.workspaceId, "Chooser lifecycle task", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await writeGroupingState(apiClient, seedData.workspaceId, {
    snapshot: {
      version: 1,
      revision: "chooser-lifecycle-r1",
      complete: true,
      groups: [{ id: "only", label: "Lifecycle labels" }],
      memberships: [{ task_id: task.id, group_id: "only" }],
    },
  });
  await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: true });
  await testPage.goto(`/t/${task.id}`);
  await testPage.getByTestId("mobile-task-picker-trigger").tap();
  const tasks = testPage.getByRole("dialog", { name: "Tasks", exact: true });
  const trigger = tasks.getByRole("button", { name: "Plugin groupings", exact: true });
  const chooser = testPage.getByRole("dialog", { name: "Plugin groupings", exact: true });
  await trigger.tap();
  await chooser.getByRole("button", { name: "Fixture grouping", exact: true }).tap();
  await expect(chooser).toBeHidden();
  const group = tasks.locator(
    `[data-testid="sidebar-group"][data-group-key="${groupingValueKey("only")}"]`,
  );
  await expect(group).toContainText(task.title);
  const saved = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
    seedData.workspaceId
  ];
  await trigger.tap();
  await expect(chooser).toBeVisible();
  expect((await apiClient.rawRequest("POST", `/api/plugins/${PLUGIN_ID}/disable`)).ok).toBe(true);
  await expect(chooser).toBeHidden();
  const newTask = tasks.getByRole("button", { name: "New", exact: true });
  await expect
    .poll(() => newTask.evaluate((element) => element.getBoundingClientRect().height))
    .toBeGreaterThanOrEqual(44);
  await expect(tasks.getByTestId("sidebar-task-page-load-error")).toBeVisible({ timeout: 35_000 });
  const retained = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
    seedData.workspaceId
  ];
  expect(retained.active_view_id).toBe(saved.active_view_id);
  expect(retained.views.find((view) => view.id === retained.active_view_id)?.group).toBe(
    SIDEBAR_GROUPING,
  );
  expect((await apiClient.rawRequest("POST", `/api/plugins/${PLUGIN_ID}/enable`)).ok).toBe(true);
  await expect(group).toContainText(task.title);
  await expect(tasks.getByTestId("sidebar-task-page-load-error")).toBeHidden();
});

for (const finish of ["discard", "save as", "overwrite"] as const) {
  test(`phone changed-group draft disclosure survives ${finish} with touch controls`, async ({
    testPage,
    apiClient,
    seedData,
  }, info) => {
    test.setTimeout(120000);
    await installFixturePlugin(testPage);
    await exerciseChangedGroupingDraft({
      page: testPage,
      api: apiClient,
      seed: seedData,
      mobile: true,
      finish,
      info,
    });
  });
}
