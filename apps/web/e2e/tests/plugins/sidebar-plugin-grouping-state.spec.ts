import { expect, test } from "../../fixtures/test-base";
import { installFixturePlugin, uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import {
  SIDEBAR_GROUPING,
  groupingValueKey,
  saveGroupingView,
  writeGroupingState,
} from "./sidebar-plugin-grouping-helpers";
import { SidebarFilterPopoverPage } from "../../pages/sidebar-filter-popover";
import { exerciseChangedGroupingDraft } from "./sidebar-grouping-draft-helpers";
import type { Route } from "@playwright/test";
import { dwell } from "../../helpers/causal-waits";

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

for (const finish of ["discard", "save as", "overwrite"] as const) {
  test(`changed-group draft disclosure survives ${finish} without mutating its saved base`, async ({
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
      mobile: false,
      finish,
      info,
    });
  });
}

test("queued disclosure preserves another tab's added and deleted saved views", async ({
  testPage,
  apiClient,
  seedData,
}, info) => {
  test.setTimeout(120000);
  await installFixturePlugin(testPage);
  const baseline = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
    seedData.workspaceId
  ];
  const released = Promise.withResolvers<void>();
  const arrived = Promise.withResolvers<void>();
  let held = false;
  const route = async (request: Route) => {
    if (
      !held &&
      request.request().method() === "PATCH" &&
      request.request().postDataJSON()?.sidebar_view_state
    ) {
      held = true;
      arrived.resolve();
      await released.promise;
    }
    await request.continue();
  };
  try {
    const root = await apiClient.createTask(seedData.workspaceId, "Queued disclosure root", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await writeGroupingState(apiClient, seedData.workspaceId, {
      snapshot: {
        version: 1,
        revision: "queued",
        complete: true,
        groups: [{ id: "keep", label: "Keep" }],
        memberships: [{ task_id: root.id, group_id: "keep" }],
      },
    });
    await saveGroupingView(apiClient, seedData.workspaceId);
    const current = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
      seedData.workspaceId
    ];
    const source = current.views.find((view) => view.id === current.active_view_id)!;
    await apiClient.saveUserSettings({
      sidebar_view_state: {
        workspace_id: seedData.workspaceId,
        views_upsert: [{ ...source, id: "remote-delete", name: "Remote deletion" }],
      },
    });
    await testPage.goto(`/?workspaceId=${seedData.workspaceId}`);
    const header = testPage
      .getByTestId("task-sidebar")
      .locator(
        `[data-testid="sidebar-group-header"][data-group-key="${groupingValueKey("keep")}"]`,
      );
    await expect(header).toHaveAttribute("aria-expanded", "true");
    await testPage.route("**/api/v1/user/settings", route);
    const editor = new SidebarFilterPopoverPage(testPage);
    await editor.open();
    await editor.openGroupSettings();
    await editor.popover
      .getByRole("switch", { name: "Indent grouped tasks", exact: true })
      .uncheck();
    await arrived.promise;
    await editor.close();
    await header.click();
    await expect(header).toHaveAttribute("aria-expanded", "false");
    await apiClient.saveUserSettings({
      sidebar_view_state: {
        workspace_id: seedData.workspaceId,
        view_ids_delete: ["remote-delete"],
        views_upsert: [{ ...source, id: "remote-add", name: "Remote addition" }],
      },
    });
    released.resolve();
    await expect
      .poll(async () => {
        const state = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
          seedData.workspaceId
        ];
        return [
          state.views.some((view) => view.id === "remote-add"),
          state.views.some((view) => view.id === "remote-delete"),
          state.draft?.group_indent,
          state.views.find((view) => view.id === source.id)?.collapsed_groups,
        ];
      })
      .toEqual([true, false, false, [groupingValueKey("keep")]]);
    await testPage.reload();
    await expect(header).toHaveAttribute("aria-expanded", "false");
    await editor.open();
    await editor.openGroupSettings();
    await expect(
      editor.popover.getByRole("switch", { name: "Indent grouped tasks", exact: true }),
    ).not.toBeChecked();
    await editor.close();
    await editor.openViewPicker();
    await expect(editor.chipByName("Remote addition")).toBeVisible();
    await expect(editor.chipByName("Remote deletion")).toHaveCount(0);
    await testPage.screenshot({
      path: info.outputPath("concurrent-views-preserved.png"),
      animations: "disabled",
    });
    await editor.closeViewPicker();
  } finally {
    released.resolve();
    await testPage.unroute("**/api/v1/user/settings", route);
    if (baseline)
      await apiClient.saveUserSettings({
        sidebar_view_state: { workspace_id: seedData.workspaceId, ...baseline },
      });
  }
});

test("failed retired-disclosure cleanup offers Retry without an automatic write loop", async ({
  testPage,
  apiClient,
  seedData,
}, info) => {
  test.setTimeout(120000);
  await installFixturePlugin(testPage);
  const baseline = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
    seedData.workspaceId
  ];
  let fail = true;
  let writes = 0;
  const route = async (request: Route) => {
    const patch =
      request.request().method() === "PATCH"
        ? request.request().postDataJSON()?.sidebar_view_state
        : null;
    if (patch?.views_update?.some((view: { id: string }) => view.id === "complete-plugin-view")) {
      writes++;
      if (fail) {
        await request.fulfill({
          status: 503,
          contentType: "application/json",
          body: JSON.stringify({ error: "Settings write unavailable" }),
        });
        return;
      }
    }
    await request.continue();
  };
  try {
    const root = await apiClient.createTask(seedData.workspaceId, "Cleanup recovery root", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await writeGroupingState(apiClient, seedData.workspaceId, {
      snapshot: {
        version: 1,
        revision: "retry",
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
    const current = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
      seedData.workspaceId
    ];
    const source = current.views.find((view) => view.id === current.active_view_id)!;
    const kept = groupingValueKey("keep");
    const keys = [
      ...Array.from({ length: 6000 }, (_, i) => `plugin-group:${i.toString(16).padStart(64, "0")}`),
      kept,
    ];
    await apiClient.saveUserSettings({
      sidebar_view_state: {
        workspace_id: seedData.workspaceId,
        views_upsert: [{ ...source, collapsed_groups: keys }],
      },
    });
    await testPage.route("**/api/v1/user/settings", route);
    await testPage.goto(`/?workspaceId=${seedData.workspaceId}`);
    const sidebar = testPage.getByTestId("task-sidebar");
    const error = sidebar.getByTestId("sidebar-task-page-load-error");
    await expect(error).toBeVisible();
    const retry = error.getByRole("button", { name: "Retry", exact: true });
    await expect(retry).toBeEnabled();
    await expect(sidebar.locator(`[data-task-row-id="${root.id}"]`)).toHaveCount(0);
    await dwell(
      testPage,
      3000,
      "negative-assertion",
      "Observe beyond the first automatic read-retry interval while settings writes remain unavailable.",
    );
    expect(writes).toBe(1);
    expect(
      (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
        seedData.workspaceId
      ].views.find((view) => view.id === source.id)?.collapsed_groups,
    ).toEqual(keys);
    await testPage.screenshot({
      path: info.outputPath("disclosure-write-retry.png"),
      animations: "disabled",
    });
    fail = false;
    await retry.click();
    await expect(error).toBeHidden();
    await expect(
      sidebar.locator(`[data-testid="sidebar-group-header"][data-group-key="${kept}"]`),
    ).toHaveAttribute("aria-expanded", "false");
    await expect(sidebar.locator(`[data-task-row-id="${root.id}"]`)).toHaveCount(1);
    await expect
      .poll(
        async () =>
          (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
            seedData.workspaceId
          ].views.find((view) => view.id === source.id)?.collapsed_groups,
      )
      .toEqual([kept]);
    expect(writes).toBe(2);
    await testPage.reload();
    await expect(sidebar.locator(`[data-task-row-id="${root.id}"]`)).toHaveCount(1);
    await testPage.screenshot({
      path: info.outputPath("disclosure-write-recovered.png"),
      animations: "disabled",
    });
  } finally {
    fail = false;
    await testPage.unroute("**/api/v1/user/settings", route);
    if (baseline)
      await apiClient.saveUserSettings({
        sidebar_view_state: { workspace_id: seedData.workspaceId, ...baseline },
      });
  }
});

test("ambiguous complete provider JSON is unavailable instead of Untagged", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  await installFixturePlugin(testPage);
  const initial = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
    seedData.workspaceId
  ];
  const root = await apiClient.createTask(seedData.workspaceId, "Strict wire root", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const pair = { task_id: root.id, group_id: "keep" };
  const healthy = {
    version: 1,
    revision: "wire",
    complete: true,
    groups: [{ id: "keep", label: "Keep" }],
    memberships: [pair, pair],
  };
  const header = testPage
    .getByTestId("task-sidebar")
    .locator(`[data-testid="sidebar-group-header"][data-group-key="${groupingValueKey("keep")}"]`);
  try {
    await writeGroupingState(apiClient, seedData.workspaceId, { snapshot: healthy });
    await saveGroupingView(apiClient, seedData.workspaceId);
    await testPage.goto(`/?workspaceId=${seedData.workspaceId}`);
    await expect(header).toBeVisible();
    for (const field of ["memberships", "MEMBERSHIPS", "groups"]) {
      const replacement = field === "groups" ? healthy.groups : [];
      const raw =
        JSON.stringify(healthy).slice(0, -1) + `,"${field}":${JSON.stringify(replacement)}}`;
      await writeGroupingState(apiClient, seedData.workspaceId, { snapshot_raw: raw });
      const response = await apiClient.rawRequest(
        "POST",
        `/api/v1/workspaces/${seedData.workspaceId}/sidebar/query`,
        { group: SIDEBAR_GROUPING, page: 1, page_size: 100 },
      );
      expect(response.status).toBe(503);
      const body = await response.json();
      expect(body.error_code).toBe("sidebar_grouping_unavailable");
      expect(body).not.toHaveProperty("entries");
      await testPage.reload();
      await expect(
        testPage.getByTestId("task-sidebar").getByRole("button", { name: "Retry", exact: true }),
      ).toBeVisible({ timeout: 45_000 });
      await expect(
        testPage.getByTestId("task-sidebar").getByTestId("sidebar-group-header"),
      ).toHaveCount(0);
    }
    await writeGroupingState(apiClient, seedData.workspaceId, { snapshot: healthy });
    await testPage.reload();
    await expect(header).toBeVisible();
  } finally {
    await testPage.close();
    await apiClient.saveUserSettings({
      sidebar_view_state: { workspace_id: seedData.workspaceId, ...initial },
    });
  }
});
