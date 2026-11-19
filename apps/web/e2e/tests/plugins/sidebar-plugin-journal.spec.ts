import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { installFixturePlugin, uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import { SidebarFilterPopoverPage } from "../../pages/sidebar-filter-popover";
import type { Page, Route } from "@playwright/test";
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

async function preferences(api: ApiClient, workspaceId: string) {
  return (await api.getUserSettings()).settings.sidebar_views_by_workspace[workspaceId];
}

function header(page: Page, id: string) {
  return page
    .getByTestId("task-sidebar")
    .locator(`[data-testid="sidebar-group-header"][data-group-key="${groupingValueKey(id)}"]`);
}

async function holdFirstSave(page: Page, afterCommit = false) {
  const arrived = Promise.withResolvers<void>();
  const released = Promise.withResolvers<void>();
  const finished = Promise.withResolvers<void>();
  let held = false;
  let writes = 0;
  let body: { views_upsert?: { id: string }[] } | undefined;
  const handler = async (route: Route) => {
    const request = route.request();
    const mutation = request.method() === "PATCH" && request.postDataJSON()?.sidebar_view_state;
    if (!mutation) return route.continue();
    writes++;
    if (held) return route.continue();
    held = true;
    body = mutation;
    try {
      const committed = afterCommit ? await route.fetch() : undefined;
      arrived.resolve();
      await released.promise;
      if (committed) await route.fulfill({ response: committed });
      else await route.continue();
    } finally {
      finished.resolve();
    }
  };
  await page.route("**/api/v1/user/settings", handler);
  return {
    arrived: arrived.promise,
    release: () => released.resolve(),
    get body() {
      return body;
    },
    get writes() {
      return writes;
    },
    dispose: async () => {
      released.resolve();
      if (held) await finished.promise;
      await page.unroute("**/api/v1/user/settings", handler);
    },
  };
}

test.beforeEach(async ({ testPage, apiClient, seedData }) => {
  test.setTimeout(120000);
  baseline = undefined;
  await installFixturePlugin(testPage);
  baseline = await preferences(apiClient, seedData.workspaceId);
  const root = await apiClient.createTask(seedData.workspaceId, "Journal root", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  rootId = root.id;
  await writeGroupingState(apiClient, seedData.workspaceId, {
    snapshot: {
      version: 1,
      complete: true,
      revision: "journal",
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
  // Stop queued browser writes before restoring shared preferences.
  await testPage.close();
  try {
    if (baseline)
      await apiClient.saveUserSettings({
        sidebar_view_state: { workspace_id: seedData.workspaceId, ...baseline },
      });
  } finally {
    await uninstallFixturePlugin(apiClient);
  }
});

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.2
test("a delayed settings ACK and queued collapse cannot resurrect another tab's deleted view", async ({
  testPage,
  apiClient,
  seedData,
}, info) => {
  await testPage.goto(`/?workspaceId=${seedData.workspaceId}`);
  await expect(header(testPage, "keep")).toHaveAttribute("aria-expanded", "true");
  const save = await holdFirstSave(testPage, true);
  try {
    await header(testPage, "keep").click();
    await save.arrived;
    await header(testPage, "other").click();
    await apiClient.saveUserSettings({
      sidebar_view_state: {
        workspace_id: seedData.workspaceId,
        view_ids_delete: ["complete-plugin-view"],
        active_view_id: "view-all-tasks",
      },
    });
    save.release();
    const editor = new SidebarFilterPopoverPage(testPage);
    await editor.expectActiveViewChip("All tasks");
    expect(
      (await preferences(apiClient, seedData.workspaceId)).views.some(
        (view) => view.id === "complete-plugin-view",
      ),
    ).toBe(false);
    await testPage.reload();
    await editor.expectActiveViewChip("All tasks");
    expect((await preferences(apiClient, seedData.workspaceId)).active_view_id).toBe(
      "view-all-tasks",
    );
    await testPage.screenshot({
      path: info.outputPath("deleted-view-stays-deleted.png"),
      animations: "disabled",
    });
  } finally {
    await save.dispose();
  }
});

for (const operation of ["append", "delete"] as const) {
  // @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.2
  test(`ordinary ${operation} merges with another tab's append without a collection conflict`, async ({
    testPage,
    apiClient,
    seedData,
  }, info) => {
    await testPage.goto(`/?workspaceId=${seedData.workspaceId}`);
    await expect(header(testPage, "keep")).toHaveAttribute("aria-expanded", "true");
    const source = (await preferences(apiClient, seedData.workspaceId)).views.find(
      (view) => view.id === "complete-plugin-view",
    )!;
    const save = await holdFirstSave(testPage);
    try {
      const editor = new SidebarFilterPopoverPage(testPage);
      if (operation === "append") await editor.beginNewView();
      else {
        await editor.open();
        await editor.beginDeleteActiveView("Complete view");
        await editor.confirmDeleteActiveView("Complete view");
      }
      await save.arrived;
      const createdId = save.body?.views_upsert?.[0]?.id;
      if (operation === "append") expect(createdId).toBeTruthy();
      await apiClient.saveUserSettings({
        sidebar_view_state: {
          workspace_id: seedData.workspaceId,
          views_upsert: [{ ...source, id: "remote-journal-view", name: "Remote view" }],
        },
      });
      save.release();
      await expect
        .poll(async () => {
          const state = await preferences(apiClient, seedData.workspaceId);
          return {
            remote: state.views.some((view) => view.id === "remote-journal-view"),
            intended:
              operation === "append"
                ? state.views.some((view) => view.id === createdId)
                : !state.views.some((view) => view.id === source.id),
          };
        })
        .toEqual({ remote: true, intended: true });
      await testPage.reload();
      await expect(
        testPage.getByTestId("task-sidebar").locator(`[data-task-row-id="${rootId}"]`).first(),
      ).toBeVisible();
      const reloaded = await preferences(apiClient, seedData.workspaceId);
      expect(reloaded.views.some((view) => view.id === "remote-journal-view")).toBe(true);
      if (operation === "append")
        expect(reloaded.views.some((view) => view.id === createdId)).toBe(true);
      else expect(reloaded.views.some((view) => view.id === source.id)).toBe(false);
      await testPage.screenshot({
        path: info.outputPath(`${operation}-preserves-remote-view.png`),
        animations: "disabled",
      });
    } finally {
      await save.dispose();
    }
  });
}

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.2
test("presentation edits queued behind draft compaction preserve bounded disclosure without another cleanup", async ({
  testPage,
  apiClient,
  seedData,
}, info) => {
  const source = (await preferences(apiClient, seedData.workspaceId)).views.find(
    (view) => view.id === "complete-plugin-view",
  )!;
  const kept = groupingValueKey("keep");
  const keys = [
    ...Array.from(
      { length: 6000 },
      (_, index) => `plugin-group:${index.toString(16).padStart(64, "0")}`,
    ),
    kept,
  ];
  await apiClient.saveUserSettings({
    sidebar_view_state: {
      workspace_id: seedData.workspaceId,
      views_upsert: [
        { ...source, group: "plugin:kandev-plugin-e2e:fixture-option-01", collapsed_groups: [] },
      ],
      draft: {
        base_view_id: source.id,
        filters: source.filters,
        sort: source.sort,
        group: SIDEBAR_GROUPING,
        group_indent: true,
        collapsed_groups: keys,
      },
    },
  });
  const save = await holdFirstSave(testPage);
  try {
    await testPage.goto(`/?workspaceId=${seedData.workspaceId}`);
    await save.arrived;
    const editor = new SidebarFilterPopoverPage(testPage);
    await editor.open();
    await editor.openGroupSettings();
    await editor.popover
      .getByRole("switch", { name: "Indent grouped tasks", exact: true })
      .uncheck();
    save.release();
    await expect
      .poll(async () => {
        const draft = (await preferences(apiClient, seedData.workspaceId)).draft;
        return { indent: draft?.group_indent, collapsed: draft?.collapsed_groups };
      })
      .toEqual({ indent: false, collapsed: [kept] });
    await editor.close();
    await expect(header(testPage, "keep")).toHaveAttribute("aria-expanded", "false");
    expect(save.writes).toBe(2);
    await testPage.reload();
    await expect(header(testPage, "keep")).toHaveAttribute("aria-expanded", "false");
    expect((await preferences(apiClient, seedData.workspaceId)).draft?.group_indent).toBe(false);
    await testPage.screenshot({
      path: info.outputPath("draft-intent-after-compaction.png"),
      animations: "disabled",
    });
  } finally {
    await save.dispose();
  }
});
