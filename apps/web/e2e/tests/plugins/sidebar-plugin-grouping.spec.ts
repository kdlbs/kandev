import { expect, test } from "../../fixtures/test-base";
import { installFixturePlugin, uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import { PLUGIN_ID } from "../../helpers/plugin-fixture";
import { SidebarFilterPopoverPage } from "../../pages/sidebar-filter-popover";
import {
  SIDEBAR_GROUPING,
  groupingValueKey,
  seedLargeDisclosure,
  seedCompleteGrouping,
  readGroupingPage,
  saveGroupingView,
  writeGroupingState,
  installUnrelatedGroupingControl,
  UNRELATED_GROUPING_PLUGIN_ID,
} from "./sidebar-plugin-grouping-helpers";

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.1
// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.2
test.afterEach(async ({ apiClient }) => {
  await uninstallFixturePlugin(apiClient);
  await apiClient.rawRequest("DELETE", `/api/plugins/${UNRELATED_GROUPING_PLUGIN_ID}`);
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

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.2
test("Save as preserves collapsed plugin groups through reload independently of its source", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await installFixturePlugin(testPage);
  await seedCompleteGrouping(apiClient, seedData);
  await saveGroupingView(apiClient, seedData.workspaceId);
  await testPage.goto(`/?workspaceId=${seedData.workspaceId}`);
  const header = testPage
    .getByTestId("task-sidebar")
    .locator(`[data-testid="sidebar-group-header"][data-group-key="${groupingValueKey("late")}"]`);
  await expect(header).toHaveAttribute("aria-expanded", "true");
  await header.click();
  await expect(header).toHaveAttribute("aria-expanded", "false");
  const editor = new SidebarFilterPopoverPage(testPage);
  await editor.open();
  await editor.popover.getByTestId("sidebar-group-settings-toggle").click();
  await editor.popover.getByRole("switch", { name: "Indent grouped tasks", exact: true }).uncheck();
  await editor.popover.getByTestId("view-save-as-button").click();
  await editor.popover.getByTestId("view-save-as-name-input").fill("Collapsed Tag copy");
  await editor.popover.getByTestId("view-save-as-confirm").click();
  await editor.close();
  await expect(testPage.getByTestId("tasks-view-picker")).toContainText("Collapsed Tag copy");
  await expect(header).toHaveAttribute("aria-expanded", "false");
  await expect
    .poll(async () => {
      const state = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
        seedData.workspaceId
      ];
      return state.views.find((view) => view.id === state.active_view_id)?.collapsed_groups;
    })
    .toEqual([groupingValueKey("late")]);
  await testPage.reload();
  await expect(header).toHaveAttribute("aria-expanded", "false");
  await header.click();
  await expect(header).toHaveAttribute("aria-expanded", "true");
  await editor.selectViewByName("Complete view");
  await expect(header).toHaveAttribute("aria-expanded", "false");
});

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.4
test("successful config restart refreshes the saved page before its next navigation", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  test.setTimeout(120_000);
  await installFixturePlugin(testPage);
  const data = await seedCompleteGrouping(apiClient, seedData);
  await saveGroupingView(apiClient, seedData.workspaceId);
  const before = await readGroupingPage(apiClient, seedData.workspaceId);
  expect(before.grouping_generation).toEqual(expect.any(String));
  await testPage.goto(`/?workspaceId=${seedData.workspaceId}`);
  await testPage
    .getByRole("separator", { name: "Resize sidebar navigation", exact: true })
    .press("Home");
  const sidebar = testPage.getByTestId("task-sidebar");
  await expect(sidebar.getByTestId("sidebar-page-controls")).toContainText("Page 1 of 2");
  const refreshed = testPage.waitForResponse(
    async (response) => {
      if (
        !response.url().endsWith(`/workspaces/${seedData.workspaceId}/sidebar/query`) ||
        response.status() !== 200 ||
        response.request().postDataJSON()?.group !== SIDEBAR_GROUPING
      ) {
        return false;
      }
      await response.finished();
      if (response.request().failure()) return false;
      const snapshot = await response.json();
      return (
        typeof snapshot.grouping_generation === "string" &&
        snapshot.grouping_generation !== before.grouping_generation
      );
    },
    { timeout: 15_000 },
  );
  const changed = await apiClient.rawRequest("PATCH", `/api/plugins/${PLUGIN_ID}`, {
    config: {
      api_token: "sidebar-config-restart-fixture-token",
      greeting: "after sidebar config restart",
    },
  });
  expect(changed.ok, await changed.text()).toBe(true);
  const current = await (await refreshed).json();
  expect(current.page).toBe(1);
  expect(current.grouping_generation).not.toBe(before.grouping_generation);
  await sidebar.getByRole("button", { name: "Next", exact: true }).click();
  await expect(sidebar.getByTestId("sidebar-page-controls")).toContainText("Page 2 of 2");
  await expect(sidebar).toContainText(data.untagged.title);
  await testPage.screenshot({
    path: testInfo.outputPath("config-restart-next-page.png"),
    animations: "disabled",
  });
  expect(
    (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[seedData.workspaceId]
      .active_view_id,
  ).toBe("complete-plugin-view");
});

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.3
// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.4
// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.5
test("groups the whole saved forest before pagination and recovers without rewriting the view", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}, testInfo) => {
  test.setTimeout(180_000);
  await installFixturePlugin(testPage);
  await installUnrelatedGroupingControl(apiClient, backend.tmpDir);
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
    `[data-testid="sidebar-group"][data-group-key="${groupingValueKey("late")}"]`,
  );
  const bulk = sidebar.locator(
    `[data-testid="sidebar-group"][data-group-key="${groupingValueKey("bulk")}"]`,
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
  for (const operation of ["config", "disable", "enable"]) {
    const refreshedCatalog = testPage.waitForResponse(
      (response) =>
        response.url().endsWith("/api/plugins/sidebar-groupings") && response.status() === 200,
    );
    const changed = await apiClient.rawRequest(
      operation === "config" ? "PATCH" : "POST",
      `/api/plugins/${UNRELATED_GROUPING_PLUGIN_ID}${operation === "config" ? "" : `/${operation}`}`,
      operation === "config" ? { config: {} } : undefined,
    );
    expect(changed.ok, await changed.text()).toBe(true);
    await (await refreshedCatalog).finished();
    await expect(sidebar.getByTestId("sidebar-page-controls")).toContainText("Page 2 of 2");
    await expect(sidebar).toContainText(data.untagged.title);
  }
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
    .toEqual([groupingValueKey("late")]);
  await testPage.reload();
  await expect(alpha.getByTestId("sidebar-group-header")).toHaveAttribute("aria-expanded", "false");
  await bulk.getByTestId("sidebar-group-header").click();
  await expect(sidebar.getByRole("button", { name: "Next", exact: true })).toHaveCount(0);
  await expect(sidebar.getByRole("button", { name: "Previous", exact: true })).toHaveCount(0);
  await expect(sidebar.getByTestId("sidebar-page-controls")).toContainText("unique tasks");
  await testPage.screenshot({
    path: testInfo.outputPath("single-page-grouping-count.png"),
    animations: "disabled",
  });
  await bulk.getByTestId("sidebar-group-header").click();
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
      `[data-testid="sidebar-group-header"][data-group-key="${SIDEBAR_GROUPING}:unassigned"]`,
    ),
  ).toContainText("Unassigned");
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

// @covers AC-UI-SIDEBAR-MULTI-TAG-001.2
// @covers AC-UI-SIDEBAR-MULTI-TAG-001.3
// @covers AC-UI-SIDEBAR-MULTI-TAG-001.4
test("overlapping combination values repeat whole trees before pagination", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  test.setTimeout(180_000);
  await installFixturePlugin(testPage);
  const data = await seedCompleteGrouping(apiClient, seedData);
  const root = data.roots[0];
  const combinations = {
    version: 1,
    revision: "combinations-r1",
    complete: true,
    groups: [
      { id: "set-b", label: '"Beta"', color: "#22aa66" },
      { id: "set-ab", label: '"Alpha" + "Beta"' },
      { id: "set-a", label: '"Alpha"', color: "#00aaff" },
    ],
    memberships: [
      { task_id: root.id, group_id: "set-a" },
      { task_id: root.id, group_id: "set-ab" },
      ...data.roots.map((task) => ({ task_id: task.id, group_id: "set-b" })),
    ],
  };
  await writeGroupingState(apiClient, seedData.workspaceId, { snapshot: combinations });
  await saveGroupingView(apiClient, seedData.workspaceId);
  const plain = await readGroupingPage(apiClient, seedData.workspaceId, { group: "none" });
  const grouped = await readGroupingPage(apiClient, seedData.workspaceId);
  expect(grouped.total_tasks).toBe(plain.total_tasks);
  // The root tree (root and child) repeats under A, A+B and B; Untagged follows.
  expect(grouped.total_visible_tasks).toBe(2 + 2 + (data.roots.length + 1) + 1);

  await testPage.goto(`/?workspaceId=${seedData.workspaceId}`);
  const sidebar = testPage.getByTestId("task-sidebar");
  const group = (id: string) =>
    sidebar.locator(`[data-testid="sidebar-group"][data-group-key="${groupingValueKey(id)}"]`);
  const rootRows = sidebar.locator(`[data-task-row-id="${root.id}"]`);
  const childRows = sidebar.locator(`[data-task-row-id="${data.child.id}"]`);
  await expect(sidebar.getByTestId("sidebar-task-item")).toHaveCount(100);
  expect(
    await sidebar
      .getByTestId("sidebar-group-header")
      .evaluateAll((headers) => headers.map((header) => header.getAttribute("data-group-key"))),
  ).toEqual(["set-a", "set-ab", "set-b"].map(groupingValueKey));
  await expect(group("set-ab").getByTestId("sidebar-group-header")).toContainText(
    '"Alpha" + "Beta"',
  );
  await expect(group("set-ab").getByTestId("sidebar-group-color")).toHaveCount(0);
  await expect(group("set-a").getByTestId("sidebar-group-color")).toHaveCount(1);
  await expect(rootRows).toHaveCount(3);
  await expect(childRows).toHaveCount(3);
  await expect(sidebar.getByTestId("sidebar-page-controls")).toContainText(
    `${plain.total_tasks} unique tasks. Page 1 of 2 (occurrences).`,
  );
  await group("set-ab")
    .locator(`[data-task-row-id="${root.id}"]`)
    .click({ modifiers: ["Control"] });
  await expect
    .poll(() =>
      rootRows.evaluateAll((rows) => rows.map((row) => row.getAttribute("aria-selected"))),
    )
    .toEqual(["true", "true", "true"]);
  await sidebar.getByRole("button", { name: "Clear selection", exact: true }).click();
  await testPage.screenshot({
    path: testInfo.outputPath("overlapping-combinations-desktop.png"),
    animations: "disabled",
  });

  const combined = group("set-ab").getByTestId("sidebar-group-header");
  await combined.click();
  await expect(combined).toHaveAttribute("aria-expanded", "false");
  await expect(childRows).toHaveCount(2);
  await expect
    .poll(async () => {
      const state = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
        seedData.workspaceId
      ];
      return state.views.find((view) => view.id === state.active_view_id)?.collapsed_groups;
    })
    .toEqual([groupingValueKey("set-ab")]);
  await testPage.reload();
  await expect(combined).toHaveAttribute("aria-expanded", "false");

  // Removing B from the root drops it from B and A+B; the unchanged A survives.
  await writeGroupingState(apiClient, seedData.workspaceId, {
    snapshot: {
      ...combinations,
      revision: "combinations-r2",
      groups: combinations.groups.filter((value) => value.id !== "set-ab"),
      memberships: [
        { task_id: root.id, group_id: "set-a" },
        ...data.roots.slice(1).map((task) => ({ task_id: task.id, group_id: "set-b" })),
      ],
    },
  });
  await expect(group("set-ab")).toHaveCount(0);
  await expect(rootRows).toHaveCount(1);
  await expect(group("set-a").locator(`[data-task-row-id="${data.child.id}"]`)).toHaveCount(1);
  await expect(group("set-b").getByTestId("sidebar-task-item").first()).toHaveAttribute(
    "data-task-row-id",
    data.roots[1].id,
  );
  await sidebar.getByRole("button", { name: "Next", exact: true }).click();
  await expect(sidebar).toContainText(data.untagged.title);

  const other = await apiClient.createWorkspace("Foreign combination workspace");
  await apiClient.createWorkflow(other.id, "Foreign combination workflow", "simple");
  await writeGroupingState(apiClient, other.id, { snapshot: combinations });
  const foreign = await readGroupingPage(apiClient, other.id);
  expect(foreign.total_tasks).toBe(0);
  expect(foreign.entries.some((entry) => entry.task_id === root.id)).toBe(false);
});

test("long provider value disclosure survives the bounded public query and reload", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  await installFixturePlugin(testPage);
  const task = await seedLargeDisclosure(apiClient, seedData);
  await testPage.goto(`/?workspaceId=${seedData.workspaceId}`);
  const sidebar = testPage.getByTestId("task-sidebar");
  const first = sidebar.getByTestId("sidebar-group-header").filter({ hasText: "Long group 000" });
  await expect(first).toHaveAttribute("aria-expanded", "false");
  await testPage.reload();
  await expect(first).toHaveAttribute("aria-expanded", "false");
  await first.click();
  await expect(first).toHaveAttribute("aria-expanded", "true");
  await expect(sidebar.getByTestId("sidebar-task-item")).toContainText(task.title);
  await testPage.reload();
  await expect(first).toHaveAttribute("aria-expanded", "true");
  await testPage.screenshot({ path: testInfo.outputPath("long-disclosure.png") });
});
