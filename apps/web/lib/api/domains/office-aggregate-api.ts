import { fetchJson, type ApiRequestOptions } from "../client";
import type { RawActivityEntry } from "./office-activity-normalize";

/**
 * One workspace row of the read-only multi-workspace aggregate
 * (GET /api/v1/office/workspaces/aggregate). Mirrors the snake_case wire shape.
 */
export type WorkspaceAggregateEntry = {
  workspace_id: string;
  name: string;
  task_count: number;
  open_tasks: number;
  in_progress_tasks: number;
  blocked_tasks: number;
  done_tasks: number;
  pending_approvals: number;
  agent_count: number;
  running_agents: number;
};

export type WorkspaceAggregateWire = {
  workspaces: WorkspaceAggregateEntry[];
  recent_activity: RawActivityEntry[];
};

/** Every Office workspace the caller can reach, with counts and a merged activity feed. */
export function getWorkspaceAggregate(options?: ApiRequestOptions) {
  return fetchJson<WorkspaceAggregateWire>(`/api/v1/office/workspaces/aggregate`, options);
}
