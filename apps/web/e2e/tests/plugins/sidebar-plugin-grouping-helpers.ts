import { expect } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { PLUGIN_ID } from "../../helpers/plugin-fixture";
import type { SidebarGroupingSnapshot } from "@kdlbs/plugin-sdk";
import type { SidebarTaskPageResponse, SidebarTaskQuery } from "@/lib/types/http";

export const SIDEBAR_GROUPING = `plugin:${PLUGIN_ID}:fixture-labels`;

export async function writeGroupingState(
  api: ApiClient,
  workspaceId: string,
  state: { snapshot?: unknown; delay_ms?: number; fail?: boolean },
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
