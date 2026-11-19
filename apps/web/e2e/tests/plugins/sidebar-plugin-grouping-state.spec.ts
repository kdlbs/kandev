import { expect, test } from "../../fixtures/test-base";
import { installFixturePlugin, uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import {
  SIDEBAR_GROUPING,
  groupingValueKey,
  saveGroupingView,
  writeGroupingState,
} from "./sidebar-plugin-grouping-helpers";
import { SidebarFilterPopoverPage } from "../../pages/sidebar-filter-popover";

test.afterEach(async ({ apiClient }) => {
  await uninstallFixturePlugin(apiClient);
});

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.3
test("shift range uses the clicked later task occurrence", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120000);
  await installFixturePlugin(testPage);
  const options = { workflow_id: seedData.workflowId, workflow_step_id: seedData.startStepId };
  const [repeated, outside, anchor, tail] = await Promise.all(
    ["B repeated", "C outside", "A anchor", "D tail"].map((title) =>
      apiClient.createTask(seedData.workspaceId, title, options),
    ),
  );
  await writeGroupingState(apiClient, seedData.workspaceId, {
    snapshot: {
      version: 1,
      revision: "range",
      complete: true,
      groups: [
        { id: "a", label: "Alpha" },
        { id: "b", label: "Beta" },
      ],
      memberships: [
        { task_id: repeated.id, group_id: "a" },
        { task_id: outside.id, group_id: "a" },
        { task_id: anchor.id, group_id: "b" },
        { task_id: repeated.id, group_id: "b" },
        { task_id: tail.id, group_id: "b" },
      ],
    },
  });
  await saveGroupingView(apiClient, seedData.workspaceId);
  await testPage.goto(`/?workspaceId=${seedData.workspaceId}`);
  const group = (id: string) =>
    testPage.locator(`[data-testid="sidebar-group"][data-group-key="${groupingValueKey(id)}"]`);
  const row = (id: string, taskId: string) => group(id).locator(`[data-task-row-id="${taskId}"]`);
  await expect(row("b", repeated.id)).toBeVisible();
  await row("b", anchor.id).click({ modifiers: ["Control"] });
  await row("b", repeated.id).click({ modifiers: ["Shift"] });
  await expect(row("b", anchor.id)).toHaveAttribute("aria-selected", "true");
  await expect(row("b", repeated.id)).toHaveAttribute("aria-selected", "true");
  await expect(row("a", repeated.id)).toHaveAttribute("aria-selected", "true");
  await expect(row("a", outside.id)).not.toHaveAttribute("aria-selected", "true");
  await expect(row("b", tail.id)).not.toHaveAttribute("aria-selected", "true");
  await expect(testPage.locator('[data-task-row-id][aria-selected="true"]')).toHaveCount(3);
  await testPage.screenshot({
    path: test.info().outputPath("occurrence-range-selected.png"),
    animations: "disabled",
  });
});

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.2
test("retired group disclosure recovers through the public query and saved journal", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  test.setTimeout(120000);
  await installFixturePlugin(testPage);
  const root = await apiClient.createTask(seedData.workspaceId, "Retired disclosure root", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await writeGroupingState(apiClient, seedData.workspaceId, {
    snapshot: {
      version: 1,
      revision: "surviving",
      complete: true,
      groups: [
        { id: "keep", label: "Keep" },
        { id: "other", label: "Other" },
      ],
      memberships: [
        { task_id: root.id, group_id: "keep" },
        { task_id: root.id, group_id: "other" },
      ],
    },
  });
  await saveGroupingView(apiClient, seedData.workspaceId);
  const initial = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
    seedData.workspaceId
  ];
  const source = initial.views.find((view) => view.id === "complete-plugin-view")!;
  const kept = groupingValueKey("keep");
  const retired = Array.from(
    { length: 6000 },
    (_, i) => `plugin-group:${i.toString(16).padStart(64, "0")}`,
  );
  await apiClient.saveUserSettings({
    sidebar_view_state: {
      workspace_id: seedData.workspaceId,
      views_upsert: [{ ...source, collapsed_groups: [...retired, kept] }],
    },
  });
  await testPage.goto(`/?workspaceId=${seedData.workspaceId}`);
  const header = testPage
    .getByTestId("task-sidebar")
    .locator(`[data-testid="sidebar-group-header"][data-group-key="${kept}"]`);
  await expect(header).toHaveAttribute("aria-expanded", "false");
  await expect(
    testPage.getByTestId("task-sidebar").locator(`[data-task-row-id="${root.id}"]`),
  ).toHaveCount(1);
  await expect
    .poll(async () => {
      const state = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
        seedData.workspaceId
      ];
      return state.views.find((view) => view.id === source.id)?.collapsed_groups;
    })
    .toEqual([kept]);
  await header.click();
  await expect(header).toHaveAttribute("aria-expanded", "true");
  await expect
    .poll(async () => {
      const state = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
        seedData.workspaceId
      ];
      return state.views.find((view) => view.id === source.id)?.collapsed_groups;
    })
    .toEqual([]);
  await testPage.reload();
  await expect(header).toHaveAttribute("aria-expanded", "true");
  await expect(
    testPage.getByTestId("task-sidebar").locator(`[data-task-row-id="${root.id}"]`),
  ).toHaveCount(2);
  await testPage.screenshot({
    path: testInfo.outputPath("retired-disclosure-recovered.png"),
    animations: "disabled",
  });
});

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.2
test("normal Save as and disclosure edits remain writable after 27 large copies", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  test.setTimeout(180000);
  await installFixturePlugin(testPage);
  const root = await apiClient.createTask(seedData.workspaceId, "Large disclosure root", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const values = Array.from({ length: 1000 }, (_, i) => ({
    id: `value-${String(i).padStart(3, "0")}`,
    label: `Value ${String(i).padStart(3, "0")}`,
  }));
  const keys = values.map((value) => groupingValueKey(value.id));
  await writeGroupingState(apiClient, seedData.workspaceId, {
    snapshot: {
      version: 1,
      revision: "large",
      complete: true,
      groups: values,
      memberships: values.map((value) => ({ task_id: root.id, group_id: value.id })),
    },
  });
  for (let index = 0; index < 27; index++) {
    await apiClient.saveUserSettings({
      sidebar_view_state: {
        workspace_id: seedData.workspaceId,
        views_upsert: [
          {
            id: `large-${index}`,
            name: `Large ${index}`,
            filters: [],
            sort: { key: "title", direction: "asc" },
            group: SIDEBAR_GROUPING,
            group_indent: true,
            collapsed_groups: keys,
          },
        ],
        active_view_id: "large-0",
      },
    });
  }
  await testPage.goto(`/?workspaceId=${seedData.workspaceId}`);
  const header = testPage
    .getByTestId("task-sidebar")
    .locator(`[data-testid="sidebar-group-header"][data-group-key="${keys[0]}"]`);
  await expect(header).toHaveAttribute("aria-expanded", "false");
  const editor = new SidebarFilterPopoverPage(testPage);
  await editor.open();
  await editor.popover.getByTestId("sidebar-group-settings-toggle").click();
  await editor.popover.getByRole("switch", { name: "Indent grouped tasks", exact: true }).uncheck();
  await editor.popover.getByTestId("view-save-as-button").click();
  await editor.popover.getByTestId("view-save-as-name-input").fill("Large independent copy");
  await editor.popover.getByTestId("view-save-as-confirm").click();
  await editor.close();
  await expect(testPage.getByTestId("tasks-view-picker")).toContainText("Large independent copy");
  await expect
    .poll(async () => {
      const state = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
        seedData.workspaceId
      ];
      return state.views.find((view) => view.id === state.active_view_id)?.collapsed_groups.length;
    })
    .toBe(1000);
  await header.click();
  await expect(header).toHaveAttribute("aria-expanded", "true");
  await expect
    .poll(async () => {
      const state = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
        seedData.workspaceId
      ];
      return [
        state.views.find((view) => view.id === state.active_view_id)?.collapsed_groups.length,
        state.views.find((view) => view.id === "large-0")?.collapsed_groups.length,
      ];
    })
    .toEqual([999, 1000]);
  await testPage.reload();
  await expect(header).toHaveAttribute("aria-expanded", "true");
  await expect(
    testPage.getByTestId("task-sidebar").locator(`[data-task-row-id="${root.id}"]`),
  ).toHaveCount(1);
  await testPage.screenshot({
    path: testInfo.outputPath("large-copy-disclosure.png"),
    animations: "disabled",
  });
  await editor.selectViewByName("Large 0");
  await expect(header).toHaveAttribute("aria-expanded", "false");
});
