import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  GitLabStatus,
  GitLabConfigureTokenResponse,
  GitLabClearTokenResponse,
  GitLabConfigureHostResponse,
  TaskMR,
  TaskMRsResponse,
} from "@/lib/types/gitlab";

export async function fetchGitLabStatus(options?: ApiRequestOptions) {
  return fetchJson<GitLabStatus>("/api/v1/gitlab/status", options);
}

export async function configureGitLabToken(token: string) {
  return fetchJson<GitLabConfigureTokenResponse>("/api/v1/gitlab/token", {
    init: { method: "POST", body: JSON.stringify({ token }) },
  });
}

export async function clearGitLabToken() {
  return fetchJson<GitLabClearTokenResponse>("/api/v1/gitlab/token", {
    init: { method: "DELETE" },
  });
}

export async function configureGitLabHost(host: string) {
  return fetchJson<GitLabConfigureHostResponse>("/api/v1/gitlab/host", {
    init: { method: "POST", body: JSON.stringify({ host }) },
  });
}

/** List every MR association for tasks in a workspace, grouped by task ID. */
export async function listWorkspaceTaskMRs(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<TaskMRsResponse>(
    `/api/v1/gitlab/workspaces/${encodeURIComponent(workspaceId)}/task-mrs`,
    options,
  );
}

/** List the MRs linked to a single task. */
export async function listTaskMRs(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<{ task_mrs: TaskMR[] | null }>(
    `/api/v1/gitlab/tasks/${encodeURIComponent(taskId)}/mrs`,
    options,
  );
}

/**
 * Sync a task↔MR row from GitLab. Used by the `pr` skill after creating an MR
 * and by the topbar's manual refresh. project_path is "namespace/path".
 */
export async function syncTaskMR(
  taskId: string,
  body: { project_path: string; iid: number; repository_id?: string },
) {
  return fetchJson<TaskMR>(`/api/v1/gitlab/tasks/${encodeURIComponent(taskId)}/mrs/sync`, {
    init: { method: "POST", body: JSON.stringify(body) },
  });
}
