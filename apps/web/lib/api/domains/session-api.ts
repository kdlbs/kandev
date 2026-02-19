import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  TaskSessionsResponse,
  TaskSessionResponse,
  ListMessagesResponse,
  ListTurnsResponse,
} from "@/lib/types/http";

// Session operations
export async function listTaskSessions(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<TaskSessionsResponse>(`/api/v1/tasks/${taskId}/sessions`, options);
}

export async function fetchTaskSession(taskSessionId: string, options?: ApiRequestOptions) {
  return fetchJson<TaskSessionResponse>(`/api/v1/task-sessions/${taskSessionId}`, options);
}

export async function listTaskSessionMessages(
  taskSessionId: string,
  params?: { limit?: number; before?: string; after?: string; sort?: "asc" | "desc" },
  options?: ApiRequestOptions,
) {
  const query = new URLSearchParams();
  if (params?.limit) query.set("limit", params.limit.toString());
  if (params?.before) query.set("before", params.before);
  if (params?.after) query.set("after", params.after);
  if (params?.sort) query.set("sort", params.sort);
  const suffix = query.toString();
  const url = `/api/v1/task-sessions/${taskSessionId}/messages${suffix ? `?${suffix}` : ""}`;
  return fetchJson<ListMessagesResponse>(url, options);
}

export async function listSessionTurns(taskSessionId: string, options?: ApiRequestOptions) {
  return fetchJson<ListTurnsResponse>(`/api/v1/task-sessions/${taskSessionId}/turns`, options);
}

export async function openSessionInEditor(
  sessionId: string,
  payload: Partial<{
    editor_id: string;
    editor_type: string;
    file_path: string;
    line: number;
    column: number;
  }>,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ url?: string }>(`/api/v1/task-sessions/${sessionId}/open-editor`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function openSessionFolder(sessionId: string, options?: ApiRequestOptions) {
  return fetchJson<{ success: boolean }>(`/api/v1/task-sessions/${sessionId}/open-folder`, {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
}
