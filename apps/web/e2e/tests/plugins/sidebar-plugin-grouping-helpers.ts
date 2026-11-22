import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { expect } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { PLUGIN_ID } from "../../helpers/plugin-fixture";
import type { SidebarGroupingSnapshot } from "@kdlbs/plugin-sdk";
import type { SidebarTaskPageResponse, SidebarTaskQuery } from "@/lib/types/http";

export const SIDEBAR_GROUPING = `plugin:${PLUGIN_ID}:fixture-labels`;
export const UNRELATED_GROUPING_PLUGIN_ID = "kandev-plugin-grouping-control";

export function groupingValueKey(value: string): string {
  return (
    "plugin-group:" +
    createHash("sha256")
      .update(SIDEBAR_GROUPING + "\0" + value)
      .digest("hex")
  );
}

export async function installUnrelatedGroupingControl(api: ApiClient, tempDir: string) {
  const response = await api.rawRequest("GET", "/api/plugins");
  expect(response.ok).toBe(true);
  const body = (await response.json()) as {
    plugins: Array<{ id: string; install_path: string; runtime: unknown }>;
  };
  const source = body.plugins.find((record) => record.id === PLUGIN_ID)!;
  const stage = path.join(tempDir, ".kandev", "plugins", UNRELATED_GROUPING_PLUGIN_ID, "1.0.0");
  fs.mkdirSync(stage, { recursive: true });
  fs.cpSync(path.join(source.install_path, "server"), path.join(stage, "server"), {
    recursive: true,
  });
  fs.writeFileSync(
    path.join(stage, "manifest.yaml"),
    JSON.stringify({
      id: UNRELATED_GROUPING_PLUGIN_ID,
      api_version: 2,
      version: "1.0.0",
      display_name: "Unrelated grouping control",
      description: "Lifecycle control without sidebar providers",
      author: "kandev",
      runtime: source.runtime,
      capabilities: { state: true },
    }),
  );
  const sync = await api.rawRequest("POST", "/api/plugins/sync");
  expect(sync.ok, await sync.text()).toBe(true);
  const enabled = await api.rawRequest(
    "POST",
    `/api/plugins/${UNRELATED_GROUPING_PLUGIN_ID}/enable`,
  );
  expect(enabled.ok, await enabled.text()).toBe(true);
  await expect
    .poll(async () => {
      const current = await api.rawRequest("GET", `/api/plugins/${UNRELATED_GROUPING_PLUGIN_ID}`);
      return (await current.json()).status;
    })
    .toBe("active");
}

export async function writeGroupingState(
  api: ApiClient,
  workspaceId: string,
  state: { snapshot?: unknown; snapshot_raw?: string; delay_ms?: number; fail?: boolean },
) {
  const response = await api.rawRequest(
    "POST",
    `/api/plugins/${PLUGIN_ID}/actions/sidebar-labels-write`,
    { workspaceId, body: state },
  );
  const text = await response.text();
  expect(response.ok, text).toBe(true);
}

export async function readGroupingPage(
  api: ApiClient,
  workspaceId: string,
  options: Partial<SidebarTaskQuery> = {},
) {
  const response = await api.rawRequest("POST", `/api/v1/workspaces/${workspaceId}/sidebar/query`, {
    filters: [],
    sort: { key: "title", direction: "asc" },
    group: SIDEBAR_GROUPING,
    collapsed_group_keys: [],
    collapsed_task_ids: [],
    page: 1,
    page_size: 100,
    locale: "en",
    ...options,
  });
  const text = await response.text();
  expect(response.ok, text).toBe(true);
  return JSON.parse(text) as SidebarTaskPageResponse;
}

export async function saveGroupingView(api: ApiClient, workspaceId: string, pinnedTaskId?: string) {
  const { settings } = await api.getUserSettings();
  const state = settings.sidebar_views_by_workspace[workspaceId];
  await api.saveUserSettings({
    sidebar_view_state: {
      workspace_id: workspaceId,
      views: [
        ...state.views,
        {
          id: "complete-plugin-view",
          name: "Complete view",
          filters: [],
          sort: { key: "title", direction: "asc" },
          group: SIDEBAR_GROUPING,
          group_indent: true,
          collapsed_groups: [],
        },
      ],
      active_view_id: "complete-plugin-view",
      draft: null,
    },
    ...(pinnedTaskId
      ? { sidebar_task_prefs: { ...settings.sidebar_task_prefs, pinned_task_ids: [pinnedTaskId] } }
      : {}),
  });
}

export async function seedCompleteGrouping(
  api: ApiClient,
  seed: { workspaceId: string; workflowId: string; startStepId: string },
) {
  const roots: Array<{ id: string; title: string }> = [];
  for (let start = 0; start < 101; start += 10) {
    const batch = await Promise.all(
      Array.from({ length: Math.min(10, 101 - start) }, (_, offset) => {
        const title = `Complete grouping root ${String(start + offset).padStart(3, "0")}`;
        return api.createTask(seed.workspaceId, title, {
          workflow_id: seed.workflowId,
          workflow_step_id: seed.startStepId,
        });
      }),
    );
    roots.push(...batch);
  }
  const child = await api.createTask(seed.workspaceId, "Complete grouping child", {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
    parent_id: roots[0].id,
  });
  const untagged = await api.createTask(seed.workspaceId, "Complete grouping untagged", {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
  });
  const snapshot: SidebarGroupingSnapshot = {
    version: 1,
    revision: "r1",
    complete: true,
    groups: [
      { id: "late", label: "Alpha", color: "#00aaff" },
      { id: "bulk", label: "Zulu", color: "#22aa66" },
    ],
    memberships: [
      ...roots.map((root) => ({ task_id: root.id, group_id: "bulk" })),
      { task_id: roots[0].id, group_id: "late" },
      { task_id: roots[100].id, group_id: "late" },
    ],
  };
  await writeGroupingState(api, seed.workspaceId, { snapshot });
  return { roots, child, untagged, snapshot };
}

export async function seedLargeDisclosure(
  api: ApiClient,
  seed: { workspaceId: string; workflowId: string; startStepId: string },
) {
  const { workspaceId } = seed;
  const task = await api.createTask(workspaceId, "Long identity root", {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
  });
  const groups = Array.from({ length: 320 }, (_, index) => ({
    id: "界".repeat(84) + String(index).padStart(3, "0"),
    label: `Long group ${String(index).padStart(3, "0")}`,
  }));
  await writeGroupingState(api, workspaceId, {
    snapshot: {
      version: 1,
      complete: true,
      revision: "long",
      groups,
      memberships: groups.map((group) => ({ task_id: task.id, group_id: group.id })),
    },
  });
  await saveGroupingView(api, workspaceId);
  const { settings } = await api.getUserSettings();
  const state = settings.sidebar_views_by_workspace[workspaceId];
  const keys = groups.map((group) => groupingValueKey(group.id));
  await api.saveUserSettings({
    sidebar_view_state: {
      workspace_id: workspaceId,
      views: state.views.map((view) => ({
        ...view,
        collapsed_groups: view.id === "complete-plugin-view" ? keys : view.collapsed_groups,
      })),
      active_view_id: "complete-plugin-view",
      draft: null,
    },
  });
  const page = await readGroupingPage(api, workspaceId, { collapsed_group_keys: keys });
  expect(page.total_visible_tasks).toBe(0);
  return task;
}
