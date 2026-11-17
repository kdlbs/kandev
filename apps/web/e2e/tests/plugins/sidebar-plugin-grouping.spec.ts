import { expect, test } from "../../fixtures/test-base";
import { installFixturePlugin, uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import { PLUGIN_ID } from "../../helpers/plugin-fixture";
import {
  SIDEBAR_GROUPING,
  seedCompleteGrouping,
  readGroupingPage,
  saveGroupingView,
  writeGroupingState,
} from "./sidebar-plugin-grouping-helpers";

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.1
// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.2
test.afterEach(async ({ apiClient }) => {
  await uninstallFixturePlugin(apiClient);
});

test("complete plugin grouping is a normal saved sidebar view", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await installFixturePlugin(testPage);
  await testPage.goto(`/?workspaceId=${seedData.workspaceId}`);
  await testPage.getByTestId("tasks-view-picker").click();
  const menu = testPage.getByRole("menu");
  await expect(menu).toBeVisible();
  expect(await menu.getByRole("menuitem").allTextContents()).toContain("Fixture grouping");
  const option = menu
    .getByRole("group", { name: "Plugin groupings", exact: true })
    .getByRole("menuitem", { name: "Fixture grouping", exact: true });
  const optionBox = await option.boundingBox();
  expect(Math.abs(optionBox!.height - 28)).toBeLessThanOrEqual(1);
  await testPage.keyboard.press("Escape");
  await expect(testPage.getByTestId("tasks-view-picker")).toBeFocused();
  await testPage.getByTestId("tasks-view-picker").press("ArrowDown");
  await expect(menu.getByRole("menuitem", { name: "All tasks", exact: true })).toBeFocused();
  await testPage.keyboard.press("ArrowDown");
  await expect(option).toBeFocused();
  await testPage.keyboard.press("Enter");
  await expect(testPage.getByTestId("tasks-view-picker")).toContainText("Fixture grouping");
  await expect
    .poll(async () => {
      const { settings } = await apiClient.getUserSettings();
      const state = settings.sidebar_views_by_workspace[seedData.workspaceId];
      return state.views.find((view) => view.id === state.active_view_id)?.group;
    })
    .toBe("plugin:kandev-plugin-e2e:fixture-labels");
  const saved = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
    seedData.workspaceId
  ];
  await testPage.reload();
  await expect(testPage.getByTestId("tasks-view-picker")).toContainText("Fixture grouping");
  await testPage.getByTestId("tasks-view-picker").click();
  await option.click();
  await expect
    .poll(async () => {
      const state = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
        seedData.workspaceId
      ];
      return [state.views.length, state.active_view_id];
    })
    .toEqual([saved.views.length, saved.active_view_id]);
});

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.5
test("retains boot groupings through a failed catalog read and recovers without reconnecting", async ({
  testPage,
  seedData,
}, testInfo) => {
  test.setTimeout(120_000);
  await installFixturePlugin(testPage);
  const releaseRetry = Promise.withResolvers<void>();
  const catalogPath = "**/api/plugins/sidebar-groupings";
  let reads = 0;
  await testPage.route(catalogPath, async (route) => {
    if (++reads === 1) {
      await route.fulfill({ status: 503, json: { error: "temporary catalog failure" } });
      return;
    }
    await releaseRetry.promise;
    await route.continue();
  });
  try {
    const failure = testPage.waitForResponse(
      (response) =>
        response.url().endsWith("/api/plugins/sidebar-groupings") && response.status() === 503,
    );
    await testPage.goto(`/?workspaceId=${seedData.workspaceId}`);
    await failure;
    await testPage.getByTestId("tasks-view-picker").click();
    const option = testPage
      .getByRole("group", { name: "Plugin groupings", exact: true })
      .getByRole("menuitem", { name: "Fixture grouping", exact: true });
    await expect(option).toBeVisible();
    const recovery = testPage.waitForResponse(
      (response) =>
        response.url().endsWith("/api/plugins/sidebar-groupings") && response.status() === 200,
    );
    releaseRetry.resolve();
    await recovery;
    await option.click();
    await expect(testPage.getByTestId("tasks-view-picker")).toContainText("Fixture grouping");
    await testPage.screenshot({
      path: testInfo.outputPath("catalog-recovery-desktop.png"),
      animations: "disabled",
    });
  } finally {
    releaseRetry.resolve();
    await testPage.unroute(catalogPath);
  }
});

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.3
// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.4
// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.5
test("groups the whole saved forest before pagination and recovers without rewriting the view", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  test.setTimeout(180_000);
  await installFixturePlugin(testPage);
  const data = await seedCompleteGrouping(apiClient, seedData);
  const plain = await readGroupingPage(apiClient, seedData.workspaceId, { group: "none" });
  expect(plain.entries.some((entry) => entry.task_id === data.roots[100].id)).toBe(false);
  await saveGroupingView(apiClient, seedData.workspaceId, data.roots[90].id);
  const first = await readGroupingPage(apiClient, seedData.workspaceId);
  expect(first.total_visible_tasks).toBe(plain.total_tasks + 3);
  expect(first.total_tasks).toBe(plain.total_tasks);
  expect(first.grouping_revision).toBe("r1");
  await testPage.goto(`/?workspaceId=${seedData.workspaceId}`);
  await testPage
    .getByRole("separator", { name: "Resize sidebar navigation", exact: true })
    .press("Home");
  const sidebar = testPage.getByTestId("task-sidebar");
  const alpha = sidebar.locator(
    `[data-testid="sidebar-group"][data-group-key="${SIDEBAR_GROUPING}:value:late"]`,
  );
  const bulk = sidebar.locator(
    `[data-testid="sidebar-group"][data-group-key="${SIDEBAR_GROUPING}:value:bulk"]`,
  );
  const rootSelector = `[data-task-row-id="${data.roots[0].id}"]`;
  const childSelector = `[data-task-row-id="${data.child.id}"]`;
  await expect(alpha.getByTestId("sidebar-task-item")).toHaveCount(3);
  await expect(alpha).toContainText(data.roots[100].title);
  await expect(alpha.getByTestId("sidebar-group-color")).toHaveCSS(
    "background-color",
    "rgb(0, 170, 255)",
  );
  await expect(bulk.getByTestId("sidebar-task-item").first()).toHaveAttribute(
    "data-task-row-id",
    data.roots[90].id,
  );
  await expect(sidebar.getByTestId("sidebar-page-controls")).toContainText(
    `${plain.total_tasks} unique tasks. Page 1 of 2 (occurrences).`,
  );
  await expect(sidebar.getByTestId("sidebar-task-item")).toHaveCount(100);
  await expect(sidebar.locator(childSelector)).toHaveCount(2);
  const occurrences = await sidebar
    .locator("[data-sidebar-occurrence]")
    .evaluateAll((rows) => rows.map((row) => row.getAttribute("data-sidebar-occurrence")));
  expect(new Set(occurrences).size).toBe(100);
  await alpha.locator(rootSelector).click({ modifiers: ["Control"] });
  await expect
    .poll(() =>
      sidebar
        .locator(rootSelector)
        .evaluateAll((rows) => rows.map((row) => row.getAttribute("aria-selected"))),
    )
    .toEqual(["true", "true"]);
  await sidebar.getByRole("button", { name: "Clear selection", exact: true }).click();
  await alpha.getByTestId("sidebar-subtask-toggle").click();
  await expect(sidebar.locator(childSelector)).toHaveCount(0);
  await alpha.getByTestId("sidebar-subtask-toggle").click();
  await expect(sidebar.locator(childSelector)).toHaveCount(2);
  await testPage.screenshot({
    path: testInfo.outputPath("complete-grouping-desktop-first-page.png"),
    animations: "disabled",
  });
  await sidebar.getByRole("button", { name: "Next", exact: true }).click();
  await expect(bulk.locator(`[data-task-row-id="${data.roots[100].id}"]`)).toBeVisible();
  await expect(sidebar).toContainText(data.untagged.title);
  const renamed = {
    ...data.snapshot,
    revision: "r2",
    groups: data.snapshot.groups.map((group) =>
      group.id === "late" ? { ...group, label: "Gamma", color: "#ff8800" } : group,
    ),
    memberships: [...data.snapshot.memberships, { task_id: data.roots[1].id, group_id: "late" }],
  };
  await writeGroupingState(apiClient, seedData.workspaceId, { snapshot: renamed });
  await expect(alpha.getByTestId("sidebar-group-header")).toContainText("Gamma");
  await expect(alpha.getByTestId("sidebar-group-color")).toHaveCSS(
    "background-color",
    "rgb(255, 136, 0)",
  );
  await expect(alpha.getByTestId("sidebar-task-item")).toHaveCount(4);
  await expect(bulk.getByTestId("sidebar-task-item").first()).toHaveAttribute(
    "data-task-row-id",
    data.roots[90].id,
  );
  await alpha.getByTestId("sidebar-group-header").click();
  await expect(alpha.getByTestId("sidebar-group-header")).toHaveAttribute("aria-expanded", "false");
  await expect(sidebar.locator(childSelector)).toHaveCount(1);
  await expect
    .poll(async () => {
      const state = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
        seedData.workspaceId
      ];
      return state.views.find((view) => view.id === state.active_view_id)?.collapsed_groups;
    })
    .toEqual([`${SIDEBAR_GROUPING}:value:late`]);
  await testPage.reload();
  await expect(alpha.getByTestId("sidebar-group-header")).toHaveAttribute("aria-expanded", "false");
  const disabled = await apiClient.rawRequest("POST", `/api/plugins/${PLUGIN_ID}/disable`);
  expect(disabled.ok, await disabled.text()).toBe(true);
  await expect(sidebar.getByRole("alert")).toContainText("plugin grouping", { timeout: 30_000 });
  await expect(sidebar.getByTestId("sidebar-group")).toHaveCount(0);
  await testPage.getByTestId("sidebar-filter-gear").click();
  await expect(testPage.getByTestId("sidebar-group-settings-toggle")).toContainText("unavailable");
  await testPage.getByTestId("sidebar-group-settings-toggle").click();
  await expect(testPage.getByTestId("group-key-select")).toContainText("unavailable");
  await testPage.keyboard.press("Escape");
  const selected = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
    seedData.workspaceId
  ];
  expect(selected.active_view_id).toBe("complete-plugin-view");
  expect(selected.views.find((view) => view.id === selected.active_view_id)?.group).toBe(
    SIDEBAR_GROUPING,
  );
  const enabled = await apiClient.rawRequest("POST", `/api/plugins/${PLUGIN_ID}/enable`);
  expect(enabled.ok, await enabled.text()).toBe(true);
  await expect(alpha.getByTestId("sidebar-group-header")).toHaveAttribute(
    "aria-expanded",
    "false",
    { timeout: 30_000 },
  );
  await expect(sidebar.getByRole("alert")).toBeHidden();
  await writeGroupingState(apiClient, seedData.workspaceId, {
    snapshot: { ...renamed, complete: false },
  });
  await expect(sidebar.getByRole("alert")).toContainText("plugin grouping", { timeout: 30_000 });
  await sidebar.getByRole("button", { name: "Retry", exact: true }).click();
  await expect(sidebar.getByRole("alert")).toContainText("plugin grouping", { timeout: 30_000 });
  await writeGroupingState(apiClient, seedData.workspaceId, {
    snapshot: { ...renamed, revision: "r3" },
  });
  await expect(alpha.getByTestId("sidebar-group-header")).toContainText("Gamma");
  await expect(sidebar.getByRole("alert")).toBeHidden();
  await writeGroupingState(apiClient, seedData.workspaceId, {
    snapshot: {
      version: 1,
      revision: "empty-complete",
      complete: true,
      groups: [],
      memberships: [],
    },
  });
  await expect(
    sidebar.locator(
      `[data-testid="sidebar-group-header"][data-group-key="${SIDEBAR_GROUPING}:untagged"]`,
    ),
  ).toContainText("Untagged");
  await expect(sidebar.getByRole("alert")).toBeHidden();
  expect(
    (await readGroupingPage(apiClient, seedData.workspaceId, { group: "none" })).total_tasks,
  ).toBe(plain.total_tasks);
});

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.6
test("narrow fine-pointer grouping chooser stays reachable and returns keyboard focus", async ({
  testPage,
}, testInfo) => {
  await installFixturePlugin(testPage);
  await testPage.setViewportSize({ width: 320, height: 851 });
  await testPage.goto("/");
  expect(await testPage.evaluate(() => matchMedia("(pointer: fine)").matches)).toBe(true);
  await testPage.getByTestId("app-nav-trigger").click();
  const trigger = testPage
    .getByTestId("app-nav-sheet")
    .getByRole("button", { name: "Plugin groupings", exact: true });
  expect((await trigger.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await trigger.click();
  const chooser = testPage.getByRole("dialog", { name: "Plugin groupings", exact: true });
  const last = chooser.getByTestId("sidebar-plugin-grouping").last();
  await last.scrollIntoViewIfNeeded();
  const box = await last.boundingBox();
  const labelBox = await last.locator("span").first().boundingBox();
  expect(labelBox!.y).toBeGreaterThanOrEqual(box!.y);
  expect(labelBox!.y + labelBox!.height).toBeLessThanOrEqual(box!.y + box!.height);
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(320);
  expect(box!.y + box!.height).toBeLessThanOrEqual(851);
  await testPage.screenshot({
    path: testInfo.outputPath("narrow-fine-pointer-grouping-chooser.png"),
    animations: "disabled",
  });
  await testPage.keyboard.press("Escape");
  await expect(chooser).toBeHidden();
  await expect(trigger).toBeFocused();
  expect(
    await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
  ).toBe(true);
});
