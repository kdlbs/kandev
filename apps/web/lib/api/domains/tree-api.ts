import { fetchJson, type ApiRequestOptions } from "../client";

const BASE = "/api/v1/office";

export type TreeHoldMode = "pause" | "cancel";

export type TreeHold = {
  id: string;
  workspace_id: string;
  root_task_id: string;
  mode: TreeHoldMode;
  created_at: string;
  released_at?: string;
};

export type TreePreviewTask = {
  id: string;
  title: string;
  status: string;
  depth: number;
};

export type TreePreview = {
  task_count: number;
  tasks: TreePreviewTask[];
  active_run_count: number;
  active_hold?: TreeHold;
};

export type TreeCostSummary = {
  task_id: string;
  task_count: number;
  include_descendants: boolean;
  cost_subcents: number;
  tokens_in: number;
  tokens_cached_in: number;
  tokens_out: number;
};

export function previewTaskTree(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<TreePreview>(`${BASE}/tasks/${taskId}/tree/preview`, {
    ...options,
    init: { method: "POST", ...options?.init },
  });
}

export function pauseTaskTree(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<{ hold: TreeHold }>(`${BASE}/tasks/${taskId}/tree/pause`, {
    ...options,
    init: { method: "POST", ...options?.init },
  });
}

export function resumeTaskTree(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<{ hold: TreeHold }>(`${BASE}/tasks/${taskId}/tree/resume`, {
    ...options,
    init: { method: "POST", ...options?.init },
  });
}

export function cancelTaskTree(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<{ hold: TreeHold }>(`${BASE}/tasks/${taskId}/tree/cancel`, {
    ...options,
    init: { method: "POST", ...options?.init },
  });
}

export function restoreTaskTree(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<{ hold: TreeHold }>(`${BASE}/tasks/${taskId}/tree/restore`, {
    ...options,
    init: { method: "POST", ...options?.init },
  });
}

export function getTreeCostSummary(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<TreeCostSummary>(`${BASE}/tasks/${taskId}/tree/cost-summary`, options);
}
