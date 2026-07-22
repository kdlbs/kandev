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
  GitLabConfig,
  SetGitLabConfigRequest,
  TestGitLabConnectionResult,
} from "@/lib/types/gitlab";
import { invalidateIntegrationAvailabilityAfter } from "@/lib/integrations/integration-availability-events";

type WorkspaceApiOptions = ApiRequestOptions & { workspaceId: string };

function withWorkspace(path: string, options: WorkspaceApiOptions): string {
  const separator = path.includes("?") ? "&" : "?";
  return `${path}${separator}workspace_id=${encodeURIComponent(options.workspaceId)}`;
}

function requestOptions(options: WorkspaceApiOptions): ApiRequestOptions {
  const { workspaceId: _workspaceId, ...rest } = options;
  return rest;
}

export async function fetchGitLabStatus(options: WorkspaceApiOptions) {
  return fetchJson<GitLabStatus>(
    withWorkspace("/api/v1/gitlab/status", options),
    requestOptions(options),
  );
}

export async function getGitLabConfig(options: WorkspaceApiOptions): Promise<GitLabConfig | null> {
  const config = await fetchJson<GitLabConfig | undefined>(
    withWorkspace("/api/v1/gitlab/config", options),
    requestOptions(options),
  );
  return config ?? null;
}

export async function setGitLabConfig(
  payload: SetGitLabConfigRequest,
  options: WorkspaceApiOptions,
) {
  return fetchJson<GitLabConfig>(withWorkspace("/api/v1/gitlab/config", options), {
    ...requestOptions(options),
    init: { ...(options?.init ?? {}), method: "PUT", body: JSON.stringify(payload) },
  });
}

export async function deleteGitLabConfig(options: WorkspaceApiOptions) {
  return fetchJson<{ deleted: boolean }>(withWorkspace("/api/v1/gitlab/config", options), {
    ...requestOptions(options),
    init: { ...(options?.init ?? {}), method: "DELETE" },
  });
}

export async function testGitLabConfig(
  payload: SetGitLabConfigRequest,
  options: WorkspaceApiOptions,
) {
  return fetchJson<TestGitLabConnectionResult>(
    withWorkspace("/api/v1/gitlab/config/test", options),
    {
      ...requestOptions(options),
      init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(payload) },
    },
  );
}

export async function copyGitLabConfig(targetWorkspaceId: string, options: WorkspaceApiOptions) {
  return fetchJson<GitLabConfig>(withWorkspace("/api/v1/gitlab/config/copy", options), {
    ...requestOptions(options),
    init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify({ targetWorkspaceId }) },
  });
}

export async function configureGitLabToken(token: string, options: WorkspaceApiOptions, host = "") {
  await invalidateIntegrationAvailabilityAfter(
    setGitLabConfig({ host, auth_method: "pat", token }, options),
  );
  return { configured: true } satisfies GitLabConfigureTokenResponse;
}

export async function clearGitLabToken(options: WorkspaceApiOptions) {
  await invalidateIntegrationAvailabilityAfter(deleteGitLabConfig(options));
  return { cleared: true } satisfies GitLabClearTokenResponse;
}

export async function configureGitLabHost(host: string, options: WorkspaceApiOptions) {
  const config = await invalidateIntegrationAvailabilityAfter(
    setGitLabConfig({ host, auth_method: "pat" }, options),
  );
  return { configured: true, host: config.host } satisfies GitLabConfigureHostResponse;
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

export async function createTaskMR(
  body: { task_id: string; repository_id?: string; mr_url: string },
  workspaceId: string,
) {
  const query = new URLSearchParams({ workspace_id: workspaceId });
  return fetchJson<TaskMR>(`/api/v1/gitlab/task-mrs?${query.toString()}`, {
    init: { method: "POST", body: JSON.stringify(body) },
  });
}

export async function deleteTaskMR(associationId: string, workspaceId: string) {
  const query = new URLSearchParams({ workspace_id: workspaceId });
  return fetchJson<{ deleted: boolean }>(
    `/api/v1/gitlab/task-mrs/${encodeURIComponent(associationId)}?${query.toString()}`,
    { init: { method: "DELETE" } },
  );
}

/** Search the current user's MRs. filter is one of "assigned", "authored",
 * "review_requested" (matches GitLab's `scope` query param). */
export async function searchUserMRs(params: {
  workspaceId: string;
  filter?: string;
  customQuery?: string;
  page?: number;
  perPage?: number;
}) {
  const qs = new URLSearchParams({ workspace_id: params.workspaceId });
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
  workspaceId: string;
  filter?: string;
  customQuery?: string;
  page?: number;
  perPage?: number;
}) {
  const qs = new URLSearchParams({ workspace_id: params.workspaceId });
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
  GitLabProjectMember,
  GitLabSubscriptionState,
} from "@/lib/types/gitlab";

export type GitLabMRIdentity = {
  workspaceId: string;
  project: string;
  iid: number;
  host?: string;
};

function reviewPath(path: string, identity: GitLabMRIdentity): string {
  const query = new URLSearchParams({
    workspace_id: identity.workspaceId,
    project: identity.project,
    iid: String(identity.iid),
  });
  if (identity.host) query.set("expected_host", identity.host);
  return `${path}?${query.toString()}`;
}

function reviewMutation<T>(
  path: string,
  method: "POST" | "PUT",
  identity: GitLabMRIdentity,
  body: Record<string, unknown> = {},
) {
  const query = new URLSearchParams({ workspace_id: identity.workspaceId });
  if (identity.host) query.set("expected_host", identity.host);
  return fetchJson<T>(`${path}?${query.toString()}`, {
    init: {
      method,
      body: JSON.stringify({ project: identity.project, iid: identity.iid, ...body }),
    },
  });
}

// --- Watches ---

export async function listMRWatches(
  workspaceId: string,
  filters?: { sessionId?: string; taskId?: string },
  options?: ApiRequestOptions,
) {
  const qs = new URLSearchParams({ workspace_id: workspaceId });
  if (filters?.sessionId) qs.set("session_id", filters.sessionId);
  if (filters?.taskId) qs.set("task_id", filters.taskId);
  return fetchJson<{ watches: MRWatch[] }>(`/api/v1/gitlab/watches/mr?${qs.toString()}`, options);
}

export async function deleteMRWatch(id: string, workspaceId: string) {
  return fetchJson<{ deleted: boolean }>(
    withWorkspace(`/api/v1/gitlab/watches/mr/${encodeURIComponent(id)}`, { workspaceId }),
    { init: { method: "DELETE" } },
  );
}

export type CreateReviewWatchRequest = Omit<
  ReviewWatch,
  "id" | "enabled" | "last_polled_at" | "created_at" | "updated_at"
>;
// workspace_id is fixed at creation; the backend update schema ignores it.
// Excluding it here keeps the type honest so callers don't think they can
// move a watch between workspaces by sending a different id.
export type UpdateReviewWatchRequest = Partial<Omit<CreateReviewWatchRequest, "workspace_id">> & {
  enabled?: boolean;
};

export async function listReviewWatches(workspaceId: string, options?: ApiRequestOptions) {
  const qs = new URLSearchParams({ workspace_id: workspaceId });
  return fetchJson<{ watches: ReviewWatch[] }>(
    `/api/v1/gitlab/watches/review?${qs.toString()}`,
    options,
  );
}

export async function createReviewWatch(req: CreateReviewWatchRequest) {
  return fetchJson<ReviewWatch>(
    withWorkspace(`/api/v1/gitlab/watches/review`, { workspaceId: req.workspace_id }),
    {
      init: { method: "POST", body: JSON.stringify(req) },
    },
  );
}

export async function updateReviewWatch(
  id: string,
  workspaceId: string,
  req: UpdateReviewWatchRequest,
) {
  return fetchJson<ReviewWatch>(
    withWorkspace(`/api/v1/gitlab/watches/review/${encodeURIComponent(id)}`, { workspaceId }),
    {
      init: { method: "PUT", body: JSON.stringify(req) },
    },
  );
}

export async function deleteReviewWatch(id: string, workspaceId: string) {
  return fetchJson<{ deleted: boolean }>(
    withWorkspace(`/api/v1/gitlab/watches/review/${encodeURIComponent(id)}`, { workspaceId }),
    { init: { method: "DELETE" } },
  );
}

export async function triggerReviewWatch(id: string, workspaceId: string) {
  return fetchJson<{ mrs: MR[]; count: number }>(
    withWorkspace(`/api/v1/gitlab/watches/review/${encodeURIComponent(id)}/trigger`, {
      workspaceId,
    }),
    { init: { method: "POST" } },
  );
}

export async function triggerAllReviewWatches(workspaceId: string) {
  return fetchJson<{ count: number }>(
    withWorkspace(`/api/v1/gitlab/watches/review/trigger-all`, { workspaceId }),
    {
      init: { method: "POST" },
    },
  );
}

export async function previewResetReviewWatch(id: string, workspaceId: string) {
  return fetchJson<{ taskCount: number }>(
    withWorkspace(`/api/v1/gitlab/watches/review/${encodeURIComponent(id)}/reset/preview`, {
      workspaceId,
    }),
  );
}

export async function resetReviewWatch(id: string, workspaceId: string) {
  return fetchJson<{ tasksDeleted: number }>(
    withWorkspace(`/api/v1/gitlab/watches/review/${encodeURIComponent(id)}/reset`, { workspaceId }),
    { init: { method: "POST" } },
  );
}

export type CreateIssueWatchRequest = Omit<
  IssueWatch,
  "id" | "enabled" | "last_polled_at" | "created_at" | "updated_at"
>;
// See UpdateReviewWatchRequest for the workspace_id rationale.
export type UpdateIssueWatchRequest = Partial<Omit<CreateIssueWatchRequest, "workspace_id">> & {
  enabled?: boolean;
};

export async function listIssueWatches(workspaceId: string, options?: ApiRequestOptions) {
  const qs = new URLSearchParams({ workspace_id: workspaceId });
  return fetchJson<{ watches: IssueWatch[] }>(
    `/api/v1/gitlab/watches/issue?${qs.toString()}`,
    options,
  );
}

export async function createIssueWatch(req: CreateIssueWatchRequest) {
  return fetchJson<IssueWatch>(
    withWorkspace(`/api/v1/gitlab/watches/issue`, { workspaceId: req.workspace_id }),
    {
      init: { method: "POST", body: JSON.stringify(req) },
    },
  );
}

export async function updateIssueWatch(
  id: string,
  workspaceId: string,
  req: UpdateIssueWatchRequest,
) {
  return fetchJson<IssueWatch>(
    withWorkspace(`/api/v1/gitlab/watches/issue/${encodeURIComponent(id)}`, { workspaceId }),
    {
      init: { method: "PUT", body: JSON.stringify(req) },
    },
  );
}

export async function deleteIssueWatch(id: string, workspaceId: string) {
  return fetchJson<{ deleted: boolean }>(
    withWorkspace(`/api/v1/gitlab/watches/issue/${encodeURIComponent(id)}`, { workspaceId }),
    {
      init: { method: "DELETE" },
    },
  );
}

export async function triggerIssueWatch(id: string, workspaceId: string) {
  return fetchJson<{ issues: Issue[]; count: number }>(
    withWorkspace(`/api/v1/gitlab/watches/issue/${encodeURIComponent(id)}/trigger`, {
      workspaceId,
    }),
    { init: { method: "POST" } },
  );
}

export async function triggerAllIssueWatches(workspaceId: string) {
  return fetchJson<{ count: number }>(
    withWorkspace(`/api/v1/gitlab/watches/issue/trigger-all`, { workspaceId }),
    {
      init: { method: "POST" },
    },
  );
}

export async function previewResetIssueWatch(id: string, workspaceId: string) {
  return fetchJson<{ taskCount: number }>(
    withWorkspace(`/api/v1/gitlab/watches/issue/${encodeURIComponent(id)}/reset/preview`, {
      workspaceId,
    }),
  );
}

export async function resetIssueWatch(id: string, workspaceId: string) {
  return fetchJson<{ tasksDeleted: number }>(
    withWorkspace(`/api/v1/gitlab/watches/issue/${encodeURIComponent(id)}/reset`, { workspaceId }),
    { init: { method: "POST" } },
  );
}

// --- Cleanup ---

export async function cleanupReviewTasks(workspaceId: string) {
  return fetchJson<{ deleted: number }>(
    withWorkspace(`/api/v1/gitlab/cleanup/review-tasks`, { workspaceId }),
    {
      init: { method: "POST" },
    },
  );
}

export async function cleanupIssueTasks(workspaceId: string) {
  return fetchJson<{ deleted: number }>(
    withWorkspace(`/api/v1/gitlab/cleanup/issue-tasks`, { workspaceId }),
    {
      init: { method: "POST" },
    },
  );
}

// --- Projects ---

export async function listUserProjects(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ projects: GitLabProject[] }>(
    withWorkspace(`/api/v1/gitlab/projects`, { workspaceId }),
    options,
  );
}

export async function searchProjects(workspaceId: string, query: string) {
  const qs = new URLSearchParams({ workspace_id: workspaceId });
  qs.set("query", query);
  return fetchJson<{ projects: GitLabProject[] }>(
    `/api/v1/gitlab/projects/search?${qs.toString()}`,
  );
}

export async function listProjectBranches(
  workspaceId: string,
  project: string,
  options?: ApiRequestOptions,
) {
  const qs = new URLSearchParams({ workspace_id: workspaceId });
  qs.set("project", project);
  return fetchJson<{ branches: GitLabRepoBranch[] }>(
    `/api/v1/gitlab/projects/branches?${qs.toString()}`,
    options,
  );
}

export async function getProjectMergeMethods(workspaceId: string, project: string) {
  const qs = new URLSearchParams({ workspace_id: workspaceId });
  qs.set("project", project);
  return fetchJson<ProjectMergeMethods>(`/api/v1/gitlab/projects/merge-methods?${qs.toString()}`);
}

// --- MR write actions ---

export async function mergeMR(
  identity: GitLabMRIdentity & { squash?: boolean; squashCommitMessage?: string },
) {
  return reviewMutation<MR>("/api/v1/gitlab/mrs/merge", "PUT", identity, {
    method: identity.squash ? "squash" : undefined,
    squash_commit_message: identity.squashCommitMessage,
  });
}

export async function approveMR(identity: GitLabMRIdentity) {
  return reviewMutation<{ approved: boolean }>("/api/v1/gitlab/mrs/approve", "POST", identity);
}

export async function unapproveMR(identity: GitLabMRIdentity) {
  return reviewMutation<{ unapproved: boolean }>("/api/v1/gitlab/mrs/unapprove", "POST", identity);
}

export async function setMRLabels(identity: GitLabMRIdentity & { labels: string[] }) {
  return reviewMutation<{ updated: boolean }>("/api/v1/gitlab/mrs/labels", "PUT", identity, {
    labels: identity.labels,
  });
}

export async function setMRAssignees(identity: GitLabMRIdentity & { assigneeIds: number[] }) {
  return reviewMutation<{ updated: boolean }>("/api/v1/gitlab/mrs/assignees", "PUT", identity, {
    assignee_ids: identity.assigneeIds,
  });
}

export async function getMRFiles(identity: GitLabMRIdentity) {
  return fetchJson<{ files: GitLabMRFile[] }>(reviewPath("/api/v1/gitlab/mrs/files", identity));
}

export async function getMRCommits(identity: GitLabMRIdentity) {
  return fetchJson<{ commits: GitLabMRCommit[] }>(
    reviewPath("/api/v1/gitlab/mrs/commits", identity),
  );
}

export async function getMRFeedback(identity: GitLabMRIdentity) {
  return fetchJson<GitLabMRFeedback>(reviewPath("/api/v1/gitlab/mrs/feedback", identity), {
    cache: "no-store",
  });
}

export async function createMRDiscussionNote(
  identity: GitLabMRIdentity & { discussionId: string; body: string },
) {
  return reviewMutation<GitLabMRFeedback["discussions"][number]["notes"][number]>(
    "/api/v1/gitlab/mrs/discussions/notes",
    "POST",
    identity,
    { discussion_id: identity.discussionId, body: identity.body },
  );
}

export async function resolveMRDiscussion(identity: GitLabMRIdentity & { discussionId: string }) {
  return reviewMutation<{ resolved: boolean }>(
    "/api/v1/gitlab/mrs/discussions/resolve",
    "POST",
    identity,
    { discussion_id: identity.discussionId },
  );
}

export async function listProjectMembers(
  workspaceId: string,
  project: string,
  query = "",
  expectedHost?: string,
) {
  const qs = new URLSearchParams({ workspace_id: workspaceId, project });
  if (query) qs.set("query", query);
  if (expectedHost) qs.set("expected_host", expectedHost);
  return fetchJson<GitLabProjectMember[]>(`/api/v1/gitlab/projects/members?${qs.toString()}`);
}

export async function setMRReviewers(identity: GitLabMRIdentity & { reviewerIds: number[] }) {
  return reviewMutation<MR>("/api/v1/gitlab/mrs/reviewers", "PUT", identity, {
    reviewer_ids: identity.reviewerIds,
  });
}

export async function getMRSubscription(identity: GitLabMRIdentity) {
  return fetchJson<GitLabSubscriptionState>(
    reviewPath("/api/v1/gitlab/mrs/subscription", identity),
    { cache: "no-store" },
  );
}

export async function setMRSubscription(identity: GitLabMRIdentity & { subscribed: boolean }) {
  return reviewMutation<GitLabSubscriptionState>(
    "/api/v1/gitlab/mrs/subscription",
    "PUT",
    identity,
    { subscribed: identity.subscribed },
  );
}

export async function getIssueSubscription(identity: GitLabMRIdentity) {
  return fetchJson<GitLabSubscriptionState>(
    reviewPath("/api/v1/gitlab/issues/subscription", identity),
    { cache: "no-store" },
  );
}

export async function setIssueSubscription(identity: GitLabMRIdentity & { subscribed: boolean }) {
  return reviewMutation<GitLabSubscriptionState>(
    "/api/v1/gitlab/issues/subscription",
    "PUT",
    identity,
    { subscribed: identity.subscribed },
  );
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
  return fetchJson<GitLabActionPresets>(
    withWorkspace(`/api/v1/gitlab/action-presets`, { workspaceId }),
    {
      init: {
        method: "PUT",
        body: JSON.stringify(body),
      },
    },
  );
}

export async function resetActionPresets(workspaceId: string) {
  const qs = new URLSearchParams();
  qs.set("workspace_id", workspaceId);
  return fetchJson<GitLabActionPresets>(`/api/v1/gitlab/action-presets/reset?${qs.toString()}`, {
    init: { method: "POST" },
  });
}

// --- Stats ---

export async function fetchGitLabStats(workspaceId: string) {
  return fetchJson<GitLabStats>(withWorkspace(`/api/v1/gitlab/stats`, { workspaceId }), {
    cache: "no-store",
  });
}
