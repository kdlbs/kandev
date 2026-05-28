import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  GitLabStatus,
  GitLabConfigureTokenResponse,
  GitLabClearTokenResponse,
  GitLabConfigureHostResponse,
  TaskMR,
  TaskMRsResponse,
  MR,
  Issue,
  MRSearchPage,
  IssueSearchPage,
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

/** Search the current user's MRs. filter is one of "assigned", "authored",
 * "review_requested" (matches GitLab's `scope` query param). */
export async function searchUserMRs(params: {
  filter?: string;
  customQuery?: string;
  page?: number;
  perPage?: number;
}) {
  const qs = new URLSearchParams();
  if (params.filter) qs.set("filter", params.filter);
  if (params.customQuery) qs.set("custom_query", params.customQuery);
  if (params.page) qs.set("page", String(params.page));
  if (params.perPage) qs.set("per_page", String(params.perPage));
  return fetchJson<MRSearchPage>(`/api/v1/gitlab/user/mrs?${qs.toString()}`, {
    cache: "no-store",
  });
}

/** Search the current user's issues. */
export async function searchUserIssues(params: {
  filter?: string;
  customQuery?: string;
  page?: number;
  perPage?: number;
}) {
  const qs = new URLSearchParams();
  if (params.filter) qs.set("filter", params.filter);
  if (params.customQuery) qs.set("custom_query", params.customQuery);
  if (params.page) qs.set("page", String(params.page));
  if (params.perPage) qs.set("per_page", String(params.perPage));
  return fetchJson<IssueSearchPage>(`/api/v1/gitlab/user/issues?${qs.toString()}`, {
    cache: "no-store",
  });
}

export type { MR, Issue, MRSearchPage, IssueSearchPage };

// ---------------------------------------------------------------------------
// Watches / presets / write actions (parity with GitHub)
// ---------------------------------------------------------------------------

import type {
  ReviewWatch,
  IssueWatch,
  MRWatch,
  GitLabStats,
  GitLabActionPresets,
  GitLabProject,
  ProjectMergeMethods,
  GitLabMRFeedback,
  GitLabMRFile,
  GitLabMRCommit,
  GitLabRepoBranch,
} from "@/lib/types/gitlab";

// --- Watches ---

export async function listMRWatches(
  filters?: { sessionId?: string; taskId?: string },
  options?: ApiRequestOptions,
) {
  const qs = new URLSearchParams();
  if (filters?.sessionId) qs.set("session_id", filters.sessionId);
  if (filters?.taskId) qs.set("task_id", filters.taskId);
  return fetchJson<{ watches: MRWatch[] }>(
    `/api/v1/gitlab/watches/mr${qs.size > 0 ? `?${qs.toString()}` : ""}`,
    options,
  );
}

export async function deleteMRWatch(id: string) {
  return fetchJson<{ deleted: boolean }>(`/api/v1/gitlab/watches/mr/${encodeURIComponent(id)}`, {
    init: { method: "DELETE" },
  });
}

export type CreateReviewWatchRequest = Omit<
  ReviewWatch,
  "id" | "enabled" | "last_polled_at" | "created_at" | "updated_at"
>;
export type UpdateReviewWatchRequest = Partial<CreateReviewWatchRequest> & {
  enabled?: boolean;
};

export async function listReviewWatches(workspaceId?: string, options?: ApiRequestOptions) {
  const qs = new URLSearchParams();
  if (workspaceId) qs.set("workspace_id", workspaceId);
  return fetchJson<{ watches: ReviewWatch[] }>(
    `/api/v1/gitlab/watches/review${qs.size > 0 ? `?${qs.toString()}` : ""}`,
    options,
  );
}

export async function createReviewWatch(req: CreateReviewWatchRequest) {
  return fetchJson<ReviewWatch>(`/api/v1/gitlab/watches/review`, {
    init: { method: "POST", body: JSON.stringify(req) },
  });
}

export async function updateReviewWatch(id: string, req: UpdateReviewWatchRequest) {
  return fetchJson<ReviewWatch>(`/api/v1/gitlab/watches/review/${encodeURIComponent(id)}`, {
    init: { method: "PUT", body: JSON.stringify(req) },
  });
}

export async function deleteReviewWatch(id: string) {
  return fetchJson<{ deleted: boolean }>(
    `/api/v1/gitlab/watches/review/${encodeURIComponent(id)}`,
    { init: { method: "DELETE" } },
  );
}

export async function triggerReviewWatch(id: string) {
  return fetchJson<{ mrs: MR[]; count: number }>(
    `/api/v1/gitlab/watches/review/${encodeURIComponent(id)}/trigger`,
    { init: { method: "POST" } },
  );
}

export async function triggerAllReviewWatches() {
  return fetchJson<{ count: number }>(`/api/v1/gitlab/watches/review/trigger-all`, {
    init: { method: "POST" },
  });
}

export type CreateIssueWatchRequest = Omit<
  IssueWatch,
  "id" | "enabled" | "last_polled_at" | "created_at" | "updated_at"
>;
export type UpdateIssueWatchRequest = Partial<CreateIssueWatchRequest> & {
  enabled?: boolean;
};

export async function listIssueWatches(workspaceId?: string, options?: ApiRequestOptions) {
  const qs = new URLSearchParams();
  if (workspaceId) qs.set("workspace_id", workspaceId);
  return fetchJson<{ watches: IssueWatch[] }>(
    `/api/v1/gitlab/watches/issue${qs.size > 0 ? `?${qs.toString()}` : ""}`,
    options,
  );
}

export async function createIssueWatch(req: CreateIssueWatchRequest) {
  return fetchJson<IssueWatch>(`/api/v1/gitlab/watches/issue`, {
    init: { method: "POST", body: JSON.stringify(req) },
  });
}

export async function updateIssueWatch(id: string, req: UpdateIssueWatchRequest) {
  return fetchJson<IssueWatch>(`/api/v1/gitlab/watches/issue/${encodeURIComponent(id)}`, {
    init: { method: "PUT", body: JSON.stringify(req) },
  });
}

export async function deleteIssueWatch(id: string) {
  return fetchJson<{ deleted: boolean }>(`/api/v1/gitlab/watches/issue/${encodeURIComponent(id)}`, {
    init: { method: "DELETE" },
  });
}

export async function triggerIssueWatch(id: string) {
  return fetchJson<{ issues: Issue[]; count: number }>(
    `/api/v1/gitlab/watches/issue/${encodeURIComponent(id)}/trigger`,
    { init: { method: "POST" } },
  );
}

export async function triggerAllIssueWatches() {
  return fetchJson<{ count: number }>(`/api/v1/gitlab/watches/issue/trigger-all`, {
    init: { method: "POST" },
  });
}

// --- Cleanup ---

export async function cleanupReviewTasks() {
  return fetchJson<{ deleted: number }>(`/api/v1/gitlab/cleanup/review-tasks`, {
    init: { method: "POST" },
  });
}

export async function cleanupIssueTasks() {
  return fetchJson<{ deleted: number }>(`/api/v1/gitlab/cleanup/issue-tasks`, {
    init: { method: "POST" },
  });
}

// --- Projects ---

export async function listUserProjects(options?: ApiRequestOptions) {
  return fetchJson<{ projects: GitLabProject[] }>(`/api/v1/gitlab/projects`, options);
}

export async function searchProjects(query: string) {
  const qs = new URLSearchParams();
  qs.set("query", query);
  return fetchJson<{ projects: GitLabProject[] }>(
    `/api/v1/gitlab/projects/search?${qs.toString()}`,
  );
}

export async function listProjectBranches(project: string) {
  const qs = new URLSearchParams();
  qs.set("project", project);
  return fetchJson<{ branches: GitLabRepoBranch[] }>(
    `/api/v1/gitlab/projects/branches?${qs.toString()}`,
  );
}

export async function getProjectMergeMethods(project: string) {
  const qs = new URLSearchParams();
  qs.set("project", project);
  return fetchJson<ProjectMergeMethods>(`/api/v1/gitlab/projects/merge-methods?${qs.toString()}`);
}

// --- MR write actions ---

export async function mergeMR(
  project: string,
  iid: number,
  method?: string,
  squashCommitMessage?: string,
) {
  return fetchJson<MR>(`/api/v1/gitlab/mrs/merge`, {
    init: {
      method: "PUT",
      body: JSON.stringify({
        project,
        iid,
        method,
        squash_commit_message: squashCommitMessage,
      }),
    },
  });
}

export async function approveMR(project: string, iid: number) {
  return fetchJson<{ approved: boolean }>(`/api/v1/gitlab/mrs/approve`, {
    init: { method: "POST", body: JSON.stringify({ project, iid }) },
  });
}

export async function unapproveMR(project: string, iid: number) {
  return fetchJson<{ unapproved: boolean }>(`/api/v1/gitlab/mrs/unapprove`, {
    init: { method: "POST", body: JSON.stringify({ project, iid }) },
  });
}

export async function setMRLabels(project: string, iid: number, labels: string[]) {
  return fetchJson<{ updated: boolean }>(`/api/v1/gitlab/mrs/labels`, {
    init: { method: "PUT", body: JSON.stringify({ project, iid, labels }) },
  });
}

export async function setMRAssignees(project: string, iid: number, assigneeIDs: number[]) {
  return fetchJson<{ updated: boolean }>(`/api/v1/gitlab/mrs/assignees`, {
    init: { method: "PUT", body: JSON.stringify({ project, iid, assignee_ids: assigneeIDs }) },
  });
}

export async function getMRFiles(project: string, iid: number) {
  const qs = new URLSearchParams();
  qs.set("project", project);
  qs.set("iid", String(iid));
  return fetchJson<{ files: GitLabMRFile[] }>(`/api/v1/gitlab/mrs/files?${qs.toString()}`);
}

export async function getMRCommits(project: string, iid: number) {
  const qs = new URLSearchParams();
  qs.set("project", project);
  qs.set("iid", String(iid));
  return fetchJson<{ commits: GitLabMRCommit[] }>(`/api/v1/gitlab/mrs/commits?${qs.toString()}`);
}

export async function getMRFeedback(project: string, iid: number) {
  const qs = new URLSearchParams();
  qs.set("project", project);
  qs.set("iid", String(iid));
  return fetchJson<GitLabMRFeedback>(`/api/v1/gitlab/mrs/feedback?${qs.toString()}`, {
    cache: "no-store",
  });
}

// --- Action presets ---

export async function getActionPresets(workspaceId: string) {
  const qs = new URLSearchParams();
  qs.set("workspace_id", workspaceId);
  return fetchJson<GitLabActionPresets>(`/api/v1/gitlab/action-presets?${qs.toString()}`);
}

export async function updateActionPresets(
  workspaceId: string,
  body: { mr?: GitLabActionPresets["mr"]; issue?: GitLabActionPresets["issue"] },
) {
  return fetchJson<GitLabActionPresets>(`/api/v1/gitlab/action-presets`, {
    init: {
      method: "PUT",
      body: JSON.stringify({ workspace_id: workspaceId, ...body }),
    },
  });
}

export async function resetActionPresets(workspaceId: string) {
  const qs = new URLSearchParams();
  qs.set("workspace_id", workspaceId);
  return fetchJson<GitLabActionPresets>(`/api/v1/gitlab/action-presets/reset?${qs.toString()}`, {
    init: { method: "POST" },
  });
}

// --- Stats ---

export async function fetchGitLabStats() {
  return fetchJson<GitLabStats>(`/api/v1/gitlab/stats`, { cache: "no-store" });
}
