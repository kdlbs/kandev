import { fetchJson, type ApiRequestOptions } from "../client";

/**
 * v1.TaskContext mirror — keep in sync with
 * apps/backend/pkg/api/v1/task_context.go. The DTO is consumed by the
 * task detail context panel and (server-side) the prompt builder.
 *
 * Contract: AvailableDocs lists key + title + metadata only. Document
 * bodies must be fetched via the existing document endpoint (or
 * get_task_document_kandev for agents).
 */
export type TaskContextDTO = {
  task: TaskRefDTO;
  parent?: TaskRefDTO;
  children: TaskRefDTO[];
  siblings: TaskRefDTO[];
  blockers: TaskRefDTO[];
  blocked_by: TaskRefDTO[];
  available_documents: DocumentRefDTO[];
  workspace_mode?: "inherit_parent" | "new_workspace" | "shared_group" | "";
  workspace_group?: WorkspaceGroupRefDTO;
  blocked_reason?: "blockers_pending" | "workspace_restoring" | "";
  workspace_status?: "active" | "requires_configuration";
};

export type TaskRefDTO = {
  id: string;
  identifier?: string;
  title: string;
  state: string;
  workspace_id: string;
  parent_id?: string;
  assignee_label?: string;
  document_keys?: string[];
};

export type DocumentRefDTO = {
  task: TaskRefDTO;
  key: string;
  title?: string;
  type?: string;
  size_bytes?: number;
  updated_at?: string;
};

export type WorkspaceGroupRefDTO = {
  id: string;
  materialized_path?: string;
  materialized_kind?: string;
  cleanup_status?: string;
  owned_by_kandev: boolean;
  members: TaskRefDTO[];
};

/**
 * Fetches the office task-handoffs context envelope for a task. Returns
 * null when the backend has not been configured with a HandoffService
 * (legacy / tests) — the caller treats null as "feature not available"
 * and renders the prior task-detail UI.
 */
export async function getTaskContext(
  taskId: string,
  options?: ApiRequestOptions,
): Promise<TaskContextDTO | null> {
  try {
    return await fetchJson<TaskContextDTO>(`/api/v1/tasks/${taskId}/context`, options);
  } catch (err) {
    // 503 (handoff service not configured) and 404 (task missing) both
    // resolve to "no context available"; the panel falls back to its
    // pre-handoffs rendering.
    const message = err instanceof Error ? err.message : "";
    if (message.includes("503") || message.includes("404")) {
      return null;
    }
    throw err;
  }
}
