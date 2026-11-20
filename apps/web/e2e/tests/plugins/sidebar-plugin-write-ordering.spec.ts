import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { installFixturePlugin, uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import { holdSidebarSettingsWrite } from "../../helpers/user-settings-barrier";
import { SidebarFilterPopoverPage } from "../../pages/sidebar-filter-popover";
import type { Page } from "@playwright/test";
import {
  SIDEBAR_GROUPING,
  groupingValueKey,
  saveGroupingView,
  writeGroupingState,
} from "./sidebar-plugin-grouping-helpers";

type WorkspaceState = Awaited<
  ReturnType<ApiClient["getUserSettings"]>
>["settings"]["sidebar_views_by_workspace"][string];
let baseline: WorkspaceState | undefined;
let rootId = "";
const VIEW = "complete-plugin-view";
const COPY = "Concurrent copy";

async function preferences(api: ApiClient, workspace: string) {
  return (await api.getUserSettings()).settings.sidebar_views_by_workspace[workspace];
}
function header(page: Page, value: string) {
  return page
    .getByTestId("task-sidebar")
    .locator(`[data-testid="sidebar-group-header"][data-group-key="${groupingValueKey(value)}"]`);
}
function settingsResponse(page: Page) {
  return page.waitForResponse(
    (response) =>
      response.request().method() === "PATCH" && response.url().endsWith("/api/v1/user/settings"),
  );
}

test.beforeEach(async ({ testPage, apiClient, seedData }) => {
  test.setTimeout(120_000);
  baseline = undefined;
  await installFixturePlugin(testPage);
  baseline = await preferences(apiClient, seedData.workspaceId);
  const root = await apiClient.createTask(seedData.workspaceId, "Ordering root", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  rootId = root.id;
  await writeGroupingState(apiClient, seedData.workspaceId, {
    snapshot: {
      version: 1,
      complete: true,
      revision: "ordering",
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
});

test.afterEach(async ({ testPage, apiClient, seedData }) => {
  await testPage.close();
  try {
    if (baseline)
      await apiClient.saveUserSettings({
        sidebar_view_state: {
          workspace_id: seedData.workspaceId,
          ...baseline,
        },
      });
  } finally {
    await uninstallFixturePlugin(apiClient);
  }
});

for (const mode of ["overwrite", "save as"] as const) {
  test(`concurrent ${mode} transfers the latest matching draft disclosure`, async ({
    testPage,
    apiClient,
    seedData,
  }, info) => {
    const original = (await preferences(apiClient, seedData.workspaceId)).views.find(
      (view) => view.id === VIEW,
    )!;
    const source = { ...original, group: "state", collapsed_groups: ["source-owned-key"] };
    await apiClient.saveUserSettings({
      sidebar_view_state: {
        workspace_id: seedData.workspaceId,
        views_upsert: [source],
        draft: {
          base_view_id: VIEW,
          filters: source.filters,
          sort: source.sort,
          group: SIDEBAR_GROUPING,
          group_indent: true,
          collapsed_groups: [],
          task_row: source.task_row,
        },
      },
    });
    const other = await testPage.context().newPage();
    const barrier = await holdSidebarSettingsWrite(testPage);
    try {
      await testPage.goto(`/?workspaceId=${seedData.workspaceId}`);
      await other.goto(`/?workspaceId=${seedData.workspaceId}`);
      await expect(header(other, "other")).toHaveAttribute("aria-expanded", "true");
      const editor = new SidebarFilterPopoverPage(testPage);
      await editor.open();
      if (mode === "overwrite") await editor.saveOverwrite();
      else await editor.saveAs(COPY);
      await barrier.arrived;
      const acknowledged = settingsResponse(other);
      await header(other, "other").click();
      await acknowledged;
      expect(
        (await preferences(apiClient, seedData.workspaceId)).draft?.collapsed_groups,
      ).toContain(groupingValueKey("other"));
      const saved = settingsResponse(testPage);
      barrier.release();
      await saved;
      const state = await preferences(apiClient, seedData.workspaceId);
      const target = state.views.find((view) => view.id === state.active_view_id)!;
      expect(target.group).toBe(SIDEBAR_GROUPING);
      expect(target.collapsed_groups).toContain(groupingValueKey("other"));
      expect(state.draft).toBeNull();
      if (mode === "save as") expect(state.views.find((view) => view.id === VIEW)).toEqual(source);
      await testPage.reload();
      await other.reload();
      await expect(header(testPage, "other")).toHaveAttribute("aria-expanded", "false");
      await expect(header(other, "other")).toHaveAttribute("aria-expanded", "false");
      if (mode === "save as") {
        const expanded = settingsResponse(testPage);
        await header(testPage, "other").click();
        await expanded;
        expect(
          (await preferences(apiClient, seedData.workspaceId)).views.find(
            (view) => view.id === VIEW,
          ),
        ).toEqual(source);
      }
      await testPage.screenshot({
        path: info.outputPath(`fresh-draft-${mode.replace(" ", "-")}.png`),
      });
    } finally {
      await barrier.dispose();
      await other.close();
    }
  });
}

test("pending provider selection deliberately blocks dependent view interactions", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await apiClient.saveUserSettings({
    sidebar_view_state: {
      workspace_id: seedData.workspaceId,
      active_view_id: "view-all-tasks",
      draft: null,
    },
  });
  await testPage.goto(`/?workspaceId=${seedData.workspaceId}`);
  const picker = testPage.getByTestId("tasks-view-picker");
  const gear = testPage.getByTestId("sidebar-filter-gear").filter({ visible: true });
  const barrier = await holdSidebarSettingsWrite(testPage);
  try {
    await picker.click();
    await testPage
      .getByRole("group", { name: "Plugin groupings", exact: true })
      .getByRole("menuitem", { name: "Fixture grouping", exact: true })
      .click();
    await barrier.arrived;
    await expect(picker).toHaveAttribute("aria-busy", "true");
    await expect(picker).toHaveAttribute("aria-disabled", "true");
    await expect(gear).toHaveAttribute("aria-disabled", "true");
    await picker.press("Enter");
    await expect(testPage.getByRole("menu")).toBeHidden();
    await gear.press("Enter");
    await expect(testPage.getByTestId("sidebar-filter-popover")).toBeHidden();
    expect(barrier.writes).toBe(1);
    const selected = settingsResponse(testPage);
    barrier.release();
    await selected;
    await expect(picker).toContainText("Fixture grouping");
    await expect(picker).not.toHaveAttribute("aria-disabled", "true");
    const editor = new SidebarFilterPopoverPage(testPage);
    await editor.selectViewByName("All tasks");
    await expect
      .poll(async () => (await preferences(apiClient, seedData.workspaceId)).active_view_id)
      .toBe("view-all-tasks");
    await testPage.reload();
    await expect(picker).toContainText("All tasks");
  } finally {
    await barrier.dispose();
  }
});

for (const delayed of [false, true]) {
  test(`inverse disclosure survives a committed response failure with delayed WS=${delayed}`, async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    let deferUpdates = delayed;
    const deferred: Array<() => void> = [];
    if (delayed)
      await testPage.routeWebSocket("**", (route) => {
        const server = route.connectToServer();
        server.onMessage((message) => {
          let settings = false;
          try {
            settings = JSON.parse(message.toString()).action === "user.settings.updated";
          } catch {
            /* Non-JSON messages pass through. */
          }
          if (deferUpdates && settings) deferred.push(() => route.send(message));
          else route.send(message);
        });
      });
    await testPage.goto(`/?workspaceId=${seedData.workspaceId}`);
    await expect(header(testPage, "keep")).toHaveAttribute("aria-expanded", "true");
    const barrier = await holdSidebarSettingsWrite(testPage, { commit: true, responseStatus: 503 });
    try {
      await header(testPage, "keep").click();
      await barrier.arrived;
      expect(
        (await preferences(apiClient, seedData.workspaceId)).views.find((view) => view.id === VIEW)
          ?.collapsed_groups,
      ).toContain(groupingValueKey("keep"));
      await header(testPage, "keep").click();
      const successor = settingsResponse(testPage).then(async (response) => {
        if (response.status() !== 503) return response;
        return settingsResponse(testPage);
      });
      barrier.release();
      await successor;
      await expect
        .poll(
          async () =>
            (await preferences(apiClient, seedData.workspaceId)).views.find(
              (view) => view.id === VIEW,
            )?.collapsed_groups,
        )
        .not.toContain(groupingValueKey("keep"));
      deferUpdates = false;
      for (const deliver of deferred.splice(0)) deliver();
      await testPage.reload();
      await expect(header(testPage, "keep")).toHaveAttribute("aria-expanded", "true");
    } finally {
      deferUpdates = false;
      for (const deliver of deferred.splice(0)) deliver();
      await barrier.dispose();
    }
  });
}

for (const draft of [false, true]) {
  test(`duplicate-only disclosure compacts through the real query and settings path: draft=${draft}`, async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const key = groupingValueKey("keep");
    const original = (await preferences(apiClient, seedData.workspaceId)).views.find(
      (view) => view.id === VIEW,
    )!;
    const keys = Array.from({ length: 1002 }, () => key);
    await apiClient.saveUserSettings({
      sidebar_view_state: {
        workspace_id: seedData.workspaceId,
        views_upsert: [
          {
            ...original,
            group: draft ? "state" : SIDEBAR_GROUPING,
            collapsed_groups: draft ? [] : keys,
          },
        ],
        draft: draft
          ? {
              base_view_id: VIEW,
              filters: original.filters,
              sort: original.sort,
              group: SIDEBAR_GROUPING,
              group_indent: true,
              collapsed_groups: keys,
              task_row: original.task_row,
            }
          : null,
      },
    });
    const bounded: unknown[] = [];
    testPage.on("request", (request) => {
      if (request.method() !== "POST" || !request.url().endsWith("/sidebar/query")) return;
      const query = request.postDataJSON();
      if (!query.disclosure_view_id && query.collapsed_group_keys?.length === 1)
        bounded.push(query.collapsed_group_keys);
    });
    await testPage.goto(`/?workspaceId=${seedData.workspaceId}`);
    await expect
      .poll(async () => {
        const state = await preferences(apiClient, seedData.workspaceId);
        return draft
          ? state.draft?.collapsed_groups
          : state.views.find((view) => view.id === VIEW)?.collapsed_groups;
      })
      .toEqual([key]);
    await expect(header(testPage, "keep")).toHaveAttribute("aria-expanded", "false");
    await expect(header(testPage, "other")).toHaveAttribute("aria-expanded", "true");
    await expect(
      testPage.getByTestId("task-sidebar").locator(`[data-task-row-id="${rootId}"]`),
    ).toBeVisible();
    await expect.poll(() => bounded).toContainEqual([key]);
    await testPage.reload();
    await expect(header(testPage, "keep")).toHaveAttribute("aria-expanded", "false");
  });
}

for (const disclosure of [false, true]) {
  test(`Save waits for the edited draft to persist: disclosure=${disclosure}`, async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const source = (await preferences(apiClient, seedData.workspaceId)).views.find(
      (view) => view.id === VIEW,
    )!;
    await apiClient.saveUserSettings({
      sidebar_view_state: {
        workspace_id: seedData.workspaceId,
        views_upsert: [{ ...source, group: "state", collapsed_groups: [] }],
        draft: {
          base_view_id: VIEW,
          group: SIDEBAR_GROUPING,
          filters: source.filters,
          sort: source.sort,
          group_indent: true,
          collapsed_groups: [],
        },
      },
    });
    await testPage.goto(`/?workspaceId=${seedData.workspaceId}`);
    await expect(header(testPage, "keep")).toBeVisible();
    const editor = new SidebarFilterPopoverPage(testPage);
    const barrier = await holdSidebarSettingsWrite(testPage, { responseStatus: 503 });
    try {
      if (disclosure) await header(testPage, "keep").click();
      else {
        await editor.open();
        await editor.setSort("Title", source.sort.direction === "desc" ? "asc" : "desc");
      }
      await barrier.arrived;
      await editor.open();
      await expect(testPage.getByTestId("view-save-button")).toBeDisabled();
      await expect(testPage.getByTestId("view-save-as-button")).toBeDisabled();
      const rejected = settingsResponse(testPage);
      barrier.release();
      expect((await rejected).status()).toBe(503);
      await expect(testPage.getByTestId("view-save-button")).toBeEnabled();
      const state = await preferences(apiClient, seedData.workspaceId);
      expect(state.draft).not.toBeNull();
      expect(state.views.find((view) => view.id === VIEW)?.group).toBe("state");
      expect(state.views.some((view) => view.name === COPY)).toBe(false);
    } finally {
      await barrier.dispose();
    }
  });
}

test("a dispatched draft prune cannot recreate the draft after another client saves it", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const source = (await preferences(apiClient, seedData.workspaceId)).views.find(
    (view) => view.id === VIEW,
  )!;
  const keys = Array.from({ length: 1002 }, () => groupingValueKey("keep"));
  await apiClient.saveUserSettings({
    sidebar_view_state: {
      workspace_id: seedData.workspaceId,
      views_upsert: [{ ...source, group: "state", collapsed_groups: [] }],
      draft: {
        base_view_id: VIEW,
        group: SIDEBAR_GROUPING,
        filters: source.filters,
        sort: source.sort,
        collapsed_groups: keys,
      },
    },
  });
  const barrier = await holdSidebarSettingsWrite(testPage);
  try {
    await testPage.goto(`/?workspaceId=${seedData.workspaceId}`);
    await barrier.arrived;
    await apiClient.saveUserSettings({
      sidebar_view_state: {
        workspace_id: seedData.workspaceId,
        save_draft: { base_view_id: VIEW, expected_group: SIDEBAR_GROUPING, target_view_id: VIEW },
      },
    });
    const oldResponse = settingsResponse(testPage);
    barrier.release();
    const acknowledged = (await (await oldResponse).json()).settings.sidebar_views_by_workspace[
      seedData.workspaceId
    ];
    expect(acknowledged.draft).toBeNull();
    expect(
      acknowledged.views.find((view: { id: string }) => view.id === VIEW).collapsed_groups,
    ).toEqual(keys);
    await expect
      .poll(async () => {
        const state = await preferences(apiClient, seedData.workspaceId);
        return {
          draft: state.draft,
          keys: state.views.find((view) => view.id === VIEW)?.collapsed_groups,
        };
      })
      .toEqual({ draft: null, keys: [groupingValueKey("keep")] });
    await expect(header(testPage, "keep")).toHaveAttribute("aria-expanded", "false");
    await testPage.reload();
    await expect(header(testPage, "keep")).toHaveAttribute("aria-expanded", "false");
  } finally {
    await barrier.dispose();
  }
});

test("a lost reused-provider ACK cannot override a newer remote selection", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const choose = async () => {
    await testPage.getByTestId("tasks-view-picker").click();
    await testPage
      .getByRole("group", { name: "Plugin groupings", exact: true })
      .getByRole("menuitem", { name: "Fixture grouping", exact: true })
      .click();
  };
  await testPage.goto(`/?workspaceId=${seedData.workspaceId}`);
  await choose();
  await expect(testPage.getByTestId("tasks-view-picker")).toContainText("Fixture grouping");
  const reused = (await preferences(apiClient, seedData.workspaceId)).active_view_id;
  await apiClient.saveUserSettings({
    sidebar_view_state: { workspace_id: seedData.workspaceId, active_view_id: "view-all-tasks" },
  });
  await testPage.reload();
  const arrived = Promise.withResolvers<void>();
  const released = Promise.withResolvers<void>();
  let held = false;
  const routePattern = "**/api/v1/user/settings";
  await testPage.route(routePattern, async (route) => {
    if (
      route.request().method() !== "PATCH" ||
      !route.request().postDataJSON()?.sidebar_view_state?.select_plugin_grouping
    )
      return route.continue();
    if (held) return route.continue();
    held = true;
    await route.fetch();
    arrived.resolve();
    await released.promise;
    await route.abort("failed");
  });
  try {
    await choose();
    await arrived.promise;
    expect((await preferences(apiClient, seedData.workspaceId)).active_view_id).toBe(reused);
    await apiClient.saveUserSettings({
      sidebar_view_state: {
        workspace_id: seedData.workspaceId,
        views_update: [{ id: reused, name: "Remotely renamed" }],
        active_view_id: "view-all-tasks",
      },
    });
    released.resolve();
    await expect(testPage.getByTestId("tasks-view-picker")).not.toHaveAttribute(
      "aria-busy",
      "true",
    );
    await expect(testPage.getByTestId("tasks-view-picker")).toContainText("All tasks");
    await testPage.reload();
    const current = await preferences(apiClient, seedData.workspaceId);
    expect(current.active_view_id).toBe("view-all-tasks");
    expect(
      current.views.filter(
        (view) => view.generated_plugin_grouping?.reference === SIDEBAR_GROUPING,
      ),
    ).toHaveLength(1);
    await expect(testPage.getByTestId("tasks-view-picker")).toContainText("All tasks");
  } finally {
    released.resolve();
    await testPage.unroute(routePattern);
  }
});
