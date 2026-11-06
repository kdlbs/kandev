import { fetchJson, type ApiRequestOptions } from "@/lib/api/client";

export type TaskGraphMode = "subtasks" | "chain";
export type TaskGraphEdgeKind = "dependency" | "related";
export type TaskGraphEdgeStatus = "resolved" | "pending" | "failed";

export type TaskGraphNodeResponse = {
  id: string;
  title: string;
  state: string;
  parent_id?: string;
  parent_title?: string;
  depth: number;
  role: "member" | "external";
  archived: boolean;
  blocked: boolean;
  blocked_reason?: string;
};

export type TaskGraphEdgeResponse = {
  from: string;
  to: string;
  kind: TaskGraphEdgeKind;
  status?: TaskGraphEdgeStatus;
};

export type TaskGraphResponse = {
  mode: TaskGraphMode;
  available_modes: TaskGraphMode[];
  focus_task_id: string;
  root?: { id: string; title: string };
  nodes: TaskGraphNodeResponse[];
  edges: TaskGraphEdgeResponse[];
  truncated: boolean;
};

/** Reads the subtree or dependency-chain graph around one task. */
export function getTaskGraph(taskId: string, mode?: TaskGraphMode, options?: ApiRequestOptions) {
  const query = mode ? `?mode=${mode}` : "";
  return fetchJson<TaskGraphResponse>(
    `/api/v1/tasks/${encodeURIComponent(taskId)}/graph${query}`,
    options,
  );
}
