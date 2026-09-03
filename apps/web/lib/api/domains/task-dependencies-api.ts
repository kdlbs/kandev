import { ApiError, fetchJson, type ApiRequestOptions } from "@/lib/api/client";

const BASE = "/api/v1";

export type TaskDependencyRefResponse = {
  id: string;
  title?: string;
  state?: string;
  status?: "resolved" | "failed" | "pending";
};

export type TaskDependencyProjectionResponse = {
  task_id?: string;
  id?: string;
  blocked?: boolean;
  blocked_reason?: string;
  depends_on?: TaskDependencyRefResponse[];
  blocks?: TaskDependencyRefResponse[];
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
  return fetchJson<TaskDependencyProjectionResponse>(`${BASE}/tasks/${taskId}`, options);
}

export function replaceTaskDependencies(
  taskId: string,
  dependsOnTaskIDs: string[],
  options?: ApiRequestOptions,
) {
  return fetchJson<TaskDependencyProjectionResponse>(`${BASE}/tasks/${taskId}/dependencies`, {
    ...options,
    init: {
      method: "PUT",
      body: JSON.stringify({ depends_on_task_ids: dependsOnTaskIDs }),
      ...options?.init,
    },
  });
}

export function getTaskDependencyCycle(error: unknown): string[] | null {
  if (!(error instanceof ApiError) || !error.body || typeof error.body !== "object") return null;
  const cycle = (error.body as { cycle?: unknown }).cycle;
  if (!Array.isArray(cycle) || !cycle.every((id): id is string => typeof id === "string")) {
    return null;
  }
  return cycle;
}
