import { fetchJson, type ApiRequestOptions } from "@/lib/api/client";

const BASE = "/api/v1";

export type TaskDependencyRefResponse = {
  id: string;
  title?: string;
  state?: string;
  status?: "resolved" | "failed" | "pending";
};

/**
 * Reads one task's dependency projection.
 *
 * The chip prefers the board store, but a task.updated event can land before or
 * after hydration carrying no dependency fields, which leaves the store copy
 * without them. Rather than depend on WS/hydration ordering for a field the
 * server derives per read, fall back to asking for it — the same
 * fetch-your-own-data shape `PRStatusChip` already uses via `useTaskPR`.
 */
export function getTaskDependencies(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<{
    id: string;
    blocked?: boolean;
    blocked_reason?: string;
    depends_on?: TaskDependencyRefResponse[];
    blocks?: TaskDependencyRefResponse[];
  }>(`${BASE}/tasks/${taskId}`, options);
}
