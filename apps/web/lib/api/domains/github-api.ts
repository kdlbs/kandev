import { fetchJson, ApiError, type ApiRequestOptions } from "../client";
import type {
  GitHubStatusResponse,
  GitHubOrg,
  GitHubRepoInfo,
  GitHubPR,
  TaskPRsResponse,
  TaskPR,
  PRFeedback,
  PRWatchesResponse,
  ReviewWatch,
  ReviewWatchesResponse,
  CreateReviewWatchRequest,
  UpdateReviewWatchRequest,
  TriggerReviewResponse,
  PRStatsResponse,
  IssueWatch,
  IssueWatchesResponse,
  CreateIssueWatchRequest,
  UpdateIssueWatchRequest,
  TriggerIssueResponse,
  GitHubIssue,
  TaskIssueLink,
  SearchPRsResponse,
  SearchIssuesResponse,
  GitHubPRStatus,
  GitHubActionPresets,
  UpdateGitHubActionPresetsRequest,
  GitHubWorkspaceSettings,
  UpdateGitHubWorkspaceSettingsRequest,
  CleanupTasksResponse,
  MergeMethod,
  RepoMergeMethods,
  TaskCIAutomationOptions,
  TaskCIAutomationPatch,
} from "@/lib/types/github";

// Status
export async function fetchGitHubStatus(options?: ApiRequestOptions) {
  return fetchJson<GitHubStatusResponse>("/api/v1/github/status", options);
}

// Token configuration
export async function configureGitHubToken(token: string) {
  return fetchJson<{ configured: boolean }>("/api/v1/github/token", {
    init: {
      method: "POST",
      body: JSON.stringify({ token }),
    },
  });
}

export async function clearGitHubToken() {
  return fetchJson<{ cleared: boolean }>("/api/v1/github/token", {
    init: { method: "DELETE" },
  });
}

// Task PR associations
export async function listTaskPRs(taskIds: string[], options?: ApiRequestOptions) {
  const query = new URLSearchParams();
  query.set("task_ids", taskIds.join(","));
  return fetchJson<TaskPRsResponse>(`/api/v1/github/task-prs?${query.toString()}`, options);
}

export async function listWorkspaceTaskPRs(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<TaskPRsResponse>(
    `/api/v1/github/task-prs?workspace_id=${encodeURIComponent(workspaceId)}`,
    options,
  );
}

export async function getTaskPR(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<TaskPR>(`/api/v1/github/task-prs/${taskId}`, options);
}

export async function createTaskPR(
  data: { task_id: string; repository_id?: string; pr_url: string },
  options?: ApiRequestOptions,
) {
  return fetchJson<TaskPR>(`/api/v1/github/task-prs`, {
    ...options,
    init: {
      ...(options?.init ?? {}),
      method: "POST",
      body: JSON.stringify(data),
    },
  });
}

export async function linkTaskIssue(
  taskId: string,
  data: { issue: string; owner?: string; repo?: string; number?: number },
  options?: ApiRequestOptions,
) {
  return fetchJson<TaskIssueLink>(`/api/v1/github/tasks/${encodeURIComponent(taskId)}/issue`, {
    ...options,
    init: {
      ...(options?.init ?? {}),
      method: "PUT",
      body: JSON.stringify(data),
    },
  });
}

export async function unlinkTaskIssue(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<{ unlinked: boolean }>(
    `/api/v1/github/tasks/${encodeURIComponent(taskId)}/issue`,
    {
      ...options,
      init: {
        ...(options?.init ?? {}),
        method: "DELETE",
      },
    },
  );
}

export async function getTaskCIAutomationOptions(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<TaskCIAutomationOptions>(
    `/api/v1/github/tasks/${encodeURIComponent(taskId)}/ci-options`,
    options,
  );
}

export async function updateTaskCIAutomationOptions(
  taskId: string,
  patch: TaskCIAutomationPatch,
  options?: ApiRequestOptions,
) {
  return fetchJson<TaskCIAutomationOptions>(
    `/api/v1/github/tasks/${encodeURIComponent(taskId)}/ci-options`,
    {
      ...options,
      init: {
        ...(options?.init ?? {}),
        method: "PATCH",
        body: JSON.stringify(patch),
      },
    },
  );
}

// PR feedback (live from GitHub)
export async function getPRFeedback(
  owner: string,
  repo: string,
  number: number,
  options?: ApiRequestOptions,
) {
  return fetchJson<PRFeedback>(`/api/v1/github/prs/${owner}/${repo}/${number}`, options);
}

// Lightweight PR status (review + checks + mergeable), skips comments.
export async function getPRStatus(
  owner: string,
  repo: string,
  number: number,
  options?: ApiRequestOptions,
) {
  return fetchJson<GitHubPRStatus>(`/api/v1/github/prs/${owner}/${repo}/${number}/status`, options);
}

export type PRStatusRef = { owner: string; repo: string; number: number };

// Batch variant of getPRStatus: one round-trip for a whole list page. The
// backend fans out concurrently and caches per-PR, so repeat calls for the
// same page are cheap. Keys in the returned map are "<owner>/<repo>#<number>".
export async function getPRStatusesBatch(refs: PRStatusRef[], options?: ApiRequestOptions) {
  return fetchJson<{ statuses: Record<string, GitHubPRStatus> }>(`/api/v1/github/prs/statuses`, {
    ...options,
    init: {
      method: "POST",
      body: JSON.stringify({ refs }),
      ...(options?.init ?? {}),
    },
  });
}

// Submit PR review
export async function submitPRReview(
  owner: string,
  repo: string,
  number: number,
  event: "APPROVE" | "COMMENT" | "REQUEST_CHANGES",
  body?: string,
) {
  return fetchJson<{ submitted: boolean }>(
    `/api/v1/github/prs/${owner}/${repo}/${number}/reviews`,
    {
      init: {
        method: "POST",
        body: JSON.stringify({ event, body: body ?? "" }),
      },
    },
  );
}

// Merge a pull request. Omit mergeMethod to let the backend pick the first
// method the repo allows (avoids GitHub's "default to merge commit" 405 on
// squash-only / rebase-only repos).
export async function mergePR(
  owner: string,
  repo: string,
  number: number,
  mergeMethod?: MergeMethod,
) {
  return fetchJson<{ merged: boolean }>(`/api/v1/github/prs/${owner}/${repo}/${number}/merge`, {
    init: {
      method: "PUT",
      body: JSON.stringify({ merge_method: mergeMethod ?? "" }),
    },
  });
}

// Fetch the merge methods a repository allows (allow_merge_commit /
// allow_squash_merge / allow_rebase_merge). Used by the merge button to
// hide disallowed options and avoid 405s.
export async function getRepoMergeMethods(
  owner: string,
  repo: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<RepoMergeMethods>(
    `/api/v1/github/repos/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/merge-methods`,
    options,
  );
}

// PR watches
export async function listPRWatches(options?: ApiRequestOptions) {
  return fetchJson<PRWatchesResponse>("/api/v1/github/watches/pr", options);
}

export async function deletePRWatch(id: string, options?: ApiRequestOptions) {
  return fetchJson<{ success: boolean }>(`/api/v1/github/watches/pr/${id}`, {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}

// Review watches
// Omit workspaceId to fetch every watch across all workspaces.
export async function listReviewWatches(workspaceId?: string, options?: ApiRequestOptions) {
  const path = workspaceId
    ? `/api/v1/github/watches/review?workspace_id=${encodeURIComponent(workspaceId)}`
    : `/api/v1/github/watches/review`;
  return fetchJson<ReviewWatchesResponse>(path, options);
}

export async function createReviewWatch(
  payload: CreateReviewWatchRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<ReviewWatch>("/api/v1/github/watches/review", {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function updateReviewWatch(
  id: string,
  workspaceId: string,
  payload: UpdateReviewWatchRequest,
  options?: ApiRequestOptions,
) {
  const params = new URLSearchParams({ workspace_id: workspaceId });
  return fetchJson<ReviewWatch>(`/api/v1/github/watches/review/${id}?${params}`, {
    ...options,
    init: { method: "PUT", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function deleteReviewWatch(
  id: string,
  workspaceId: string,
  options?: ApiRequestOptions,
) {
  const params = new URLSearchParams({ workspace_id: workspaceId });
  return fetchJson<{ success: boolean }>(`/api/v1/github/watches/review/${id}?${params}`, {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}

export async function triggerReviewWatch(
  id: string,
  workspaceId: string,
  options?: ApiRequestOptions,
) {
  const params = new URLSearchParams({ workspace_id: workspaceId });
  return fetchJson<TriggerReviewResponse>(`/api/v1/github/watches/review/${id}/trigger?${params}`, {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
}

// previewResetReviewWatch returns how many tasks would be deleted if the
// review watch were reset. Used by the confirmation dialog.
// workspaceId is the row's owning workspace; the backend rejects mismatches
// with 404 so cross-workspace IDOR is closed off.
export async function previewResetReviewWatch(
  id: string,
  workspaceId: string,
  options?: ApiRequestOptions,
) {
  const query = new URLSearchParams({ workspace_id: workspaceId });
  return fetchJson<{ taskCount: number }>(
    `/api/v1/github/watches/review/${id}/reset/preview?${query.toString()}`,
    options,
  );
}

// resetReviewWatch deletes every task previously created by the review
// watch (including archived), wipes its dedup table, and schedules the
// watch to re-run so currently-matching PRs are queued for task creation.
export async function resetReviewWatch(
  id: string,
  workspaceId: string,
  options?: ApiRequestOptions,
) {
  const query = new URLSearchParams({ workspace_id: workspaceId });
  return fetchJson<{ tasksDeleted: number }>(
    `/api/v1/github/watches/review/${id}/reset?${query.toString()}`,
    {
      ...options,
      init: { method: "POST", ...(options?.init ?? {}) },
    },
  );
}

export async function triggerAllReviewWatches(workspaceId: string, options?: ApiRequestOptions) {
  const query = new URLSearchParams({ workspace_id: workspaceId });
  return fetchJson<TriggerReviewResponse>(
    `/api/v1/github/watches/review/trigger-all?${query.toString()}`,
    {
      ...options,
      init: { method: "POST", ...(options?.init ?? {}) },
    },
  );
}

// Accessible repos — the union of repos the authenticated user can reach
// (own + each org's), as served by `GET /api/v1/github/repos`. The
// provider-tagged shape is forward-compat for a future GitLab variant; today
// the backend only returns GitHub repos, so we stamp `provider: "github"` on
// every entry at the client boundary.
//
// `default_branch` is always present on the wire (GitHub's API returns it on
// every repo) so the picker can pre-fill the row's branch on selection
// without a round-trip. `description` is optional because the backend uses
// `omitempty` — null/empty descriptions are dropped from the JSON.
export type AccessibleRepo = {
  provider: "github" | "gitlab";
  owner: string;
  name: string;
  full_name: string;
  default_branch: string;
  description?: string;
  pushed_at?: string;
  private: boolean;
};

// Backend response shape for `GET /api/v1/github/repos` — narrow alias used
// only at the parse boundary. We keep it scoped to this module to avoid
// leaking the wire-only shape into the rest of the app.
type AccessibleReposResponse = {
  repos: Array<Omit<AccessibleRepo, "provider">>;
};

// GitHubUnavailableError signals that the backend reported GitHub is not
// configured (HTTP 503 with `code: "github_not_configured"`). Callers use the
// instanceof check to render a "Connect GitHub" CTA instead of a generic
// error toast.
export class GitHubUnavailableError extends Error {
  constructor(message = "GitHub is not configured") {
    super(message);
    this.name = "GitHubUnavailableError";
  }
}

function isGitHubNotConfigured(err: unknown): boolean {
  if (!(err instanceof ApiError)) return false;
  if (err.status !== 503) return false;
  const body = err.body;
  if (body && typeof body === "object" && "code" in body) {
    return (body as { code?: unknown }).code === "github_not_configured";
  }
  return false;
}

export async function fetchAccessibleRepos(opts: {
  q?: string;
  limit?: number;
  signal?: AbortSignal;
}): Promise<AccessibleRepo[]> {
  const params = new URLSearchParams();
  if (opts.q) params.set("q", opts.q);
  if (typeof opts.limit === "number") params.set("limit", String(opts.limit));
  const suffix = params.toString();
  const path = `/api/v1/github/repos${suffix ? `?${suffix}` : ""}`;
  try {
    const res = await fetchJson<AccessibleReposResponse>(path, {
      cache: "no-store",
      init: opts.signal ? { signal: opts.signal } : undefined,
    });
    const repos = res?.repos ?? [];
    return repos.map((r) => ({ ...r, provider: "github" as const }));
  } catch (err) {
    if (isGitHubNotConfigured(err)) {
      throw new GitHubUnavailableError(err instanceof Error ? err.message : undefined);
    }
    throw err;
  }
}

// Orgs & repo search
export async function listUserOrgs(options?: ApiRequestOptions) {
  return fetchJson<{ orgs: GitHubOrg[] }>("/api/v1/github/orgs", options);
}

export async function searchOrgRepos(org: string, query?: string, options?: ApiRequestOptions) {
  const params = new URLSearchParams({ org });
  if (query) params.set("q", query);
  return fetchJson<{ repos: GitHubRepoInfo[] }>(
    `/api/v1/github/repos/search?${params.toString()}`,
    options,
  );
}

// PR info (lightweight)
export async function fetchPRInfo(
  owner: string,
  repo: string,
  number: number,
  options?: ApiRequestOptions,
) {
  return fetchJson<GitHubPR>(
    `/api/v1/github/prs/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/${number}/info`,
    options,
  );
}

// Issue info (lightweight)
export async function fetchIssueInfo(
  owner: string,
  repo: string,
  number: number,
  options?: ApiRequestOptions,
) {
  return fetchJson<GitHubIssue>(
    `/api/v1/github/issues/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/${number}/info`,
    options,
  );
}

// Remote repo branches
export async function fetchRepoBranches(owner: string, repo: string, options?: ApiRequestOptions) {
  return fetchJson<{ branches: { name: string }[] }>(
    `/api/v1/github/repos/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/branches`,
    options,
  );
}

// Stats
export async function fetchGitHubStats(
  params?: { workspace_id?: string; start_date?: string; end_date?: string },
  options?: ApiRequestOptions,
) {
  const query = new URLSearchParams();
  if (params?.workspace_id) query.set("workspace_id", params.workspace_id);
  if (params?.start_date) query.set("start_date", params.start_date);
  if (params?.end_date) query.set("end_date", params.end_date);
  const suffix = query.toString();
  return fetchJson<PRStatsResponse>(`/api/v1/github/stats${suffix ? `?${suffix}` : ""}`, options);
}

// Issue watches
// Omit workspaceId to fetch every watch across all workspaces.
export async function listIssueWatches(workspaceId?: string, options?: ApiRequestOptions) {
  const path = workspaceId
    ? `/api/v1/github/watches/issue?workspace_id=${encodeURIComponent(workspaceId)}`
    : `/api/v1/github/watches/issue`;
  return fetchJson<IssueWatchesResponse>(path, options);
}

export async function createIssueWatch(
  payload: CreateIssueWatchRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<IssueWatch>("/api/v1/github/watches/issue", {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function updateIssueWatch(
  id: string,
  workspaceId: string,
  payload: UpdateIssueWatchRequest,
  options?: ApiRequestOptions,
) {
  const params = new URLSearchParams({ workspace_id: workspaceId });
  return fetchJson<IssueWatch>(`/api/v1/github/watches/issue/${id}?${params}`, {
    ...options,
    init: { method: "PUT", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function deleteIssueWatch(
  id: string,
  workspaceId: string,
  options?: ApiRequestOptions,
) {
  const params = new URLSearchParams({ workspace_id: workspaceId });
  return fetchJson<{ deleted: boolean }>(`/api/v1/github/watches/issue/${id}?${params}`, {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}

export async function triggerIssueWatch(
  id: string,
  workspaceId: string,
  options?: ApiRequestOptions,
) {
  const params = new URLSearchParams({ workspace_id: workspaceId });
  return fetchJson<TriggerIssueResponse>(`/api/v1/github/watches/issue/${id}/trigger?${params}`, {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
}

// previewResetIssueWatch returns how many tasks would be deleted if the
// issue watch were reset. Used by the confirmation dialog. workspaceId is
// the row's owning workspace; the backend rejects mismatches with 404.
export async function previewResetIssueWatch(
  id: string,
  workspaceId: string,
  options?: ApiRequestOptions,
) {
  const query = new URLSearchParams({ workspace_id: workspaceId });
  return fetchJson<{ taskCount: number }>(
    `/api/v1/github/watches/issue/${id}/reset/preview?${query.toString()}`,
    options,
  );
}

// resetIssueWatch deletes every task previously created by the issue
// watch (including archived), wipes its dedup table, and nulls
// last_polled_at so the next poll re-imports every currently-matching
// issue.
export async function resetIssueWatch(
  id: string,
  workspaceId: string,
  options?: ApiRequestOptions,
) {
  const query = new URLSearchParams({ workspace_id: workspaceId });
  return fetchJson<{ tasksDeleted: number }>(
    `/api/v1/github/watches/issue/${id}/reset?${query.toString()}`,
    {
      ...options,
      init: { method: "POST", ...(options?.init ?? {}) },
    },
  );
}

export async function triggerAllIssueWatches(workspaceId: string, options?: ApiRequestOptions) {
  const query = new URLSearchParams({ workspace_id: workspaceId });
  return fetchJson<TriggerIssueResponse>(
    `/api/v1/github/watches/issue/trigger-all?${query.toString()}`,
    {
      ...options,
      init: { method: "POST", ...(options?.init ?? {}) },
    },
  );
}

// Manual cleanup sweeps. The poller runs these every 5min per watch, but a
// user with a pile of legacy merged-PR tasks (created before the cleanup
// policy was in place) can invoke them on demand from the settings page.
export async function cleanupMergedReviewTasks(workspaceId?: string, options?: ApiRequestOptions) {
  const query = workspaceId ? `?${new URLSearchParams({ workspace_id: workspaceId })}` : "";
  return fetchJson<CleanupTasksResponse>(`/api/v1/github/cleanup/review-tasks${query}`, {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
}

export async function cleanupClosedIssueTasks(workspaceId?: string, options?: ApiRequestOptions) {
  const query = workspaceId ? `?${new URLSearchParams({ workspace_id: workspaceId })}` : "";
  return fetchJson<CleanupTasksResponse>(`/api/v1/github/cleanup/issue-tasks${query}`, {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
}

// User PR / issue search (for the /github page).
// Pass `query` to use a verbatim GitHub search string, or `filter` to append to
// the default (type:pr state:open / type:issue state:open).
type SearchParams = {
  query?: string;
  filter?: string;
  page?: number;
  perPage?: number;
  workspaceId?: string | null;
};

function buildSearchQuery(params: SearchParams) {
  const search = new URLSearchParams();
  if (params.query) search.set("query", params.query);
  if (params.filter) search.set("filter", params.filter);
  if (params.page && params.page > 1) search.set("page", String(params.page));
  if (params.perPage) search.set("per_page", String(params.perPage));
  if (params.workspaceId) search.set("workspace_id", params.workspaceId);
  return search.toString();
}

export async function searchUserPRs(params: SearchParams, options?: ApiRequestOptions) {
  const suffix = buildSearchQuery(params);
  return fetchJson<SearchPRsResponse>(
    `/api/v1/github/user/prs${suffix ? `?${suffix}` : ""}`,
    options,
  );
}

export async function searchUserIssues(params: SearchParams, options?: ApiRequestOptions) {
  const suffix = buildSearchQuery(params);
  return fetchJson<SearchIssuesResponse>(
    `/api/v1/github/user/issues${suffix ? `?${suffix}` : ""}`,
    options,
  );
}

// Workspace settings for GitHub repo visibility/scope.
export async function fetchGitHubWorkspaceSettings(
  workspaceId: string,
  options?: ApiRequestOptions,
) {
  const query = new URLSearchParams({ workspace_id: workspaceId });
  return fetchJson<GitHubWorkspaceSettings>(
    `/api/v1/github/workspace-settings?${query.toString()}`,
    options,
  );
}

export async function updateGitHubWorkspaceSettings(
  payload: UpdateGitHubWorkspaceSettingsRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<GitHubWorkspaceSettings>("/api/v1/github/workspace-settings", {
    ...options,
    init: { ...(options?.init ?? {}), method: "PUT", body: JSON.stringify(payload) },
  });
}

// copyGitHubWorkspaceSettings copies the per-workspace GitHub settings (repo
// scope + presets) from sourceWorkspaceId to targetWorkspaceId. GitHub auth is
// install-wide, so there are no credentials to copy.
export async function copyGitHubWorkspaceSettings(
  sourceWorkspaceId: string,
  targetWorkspaceId: string,
  options?: ApiRequestOptions,
) {
  const query = new URLSearchParams({ workspace_id: sourceWorkspaceId });
  return fetchJson<GitHubWorkspaceSettings>(
    `/api/v1/github/workspace-settings/copy?${query.toString()}`,
    {
      ...options,
      init: {
        ...(options?.init ?? {}),
        method: "POST",
        body: JSON.stringify({ targetWorkspaceId }),
      },
    },
  );
}

// Action presets (quick-launch prompts on the /github page).
export async function fetchGitHubActionPresets(workspaceId: string, options?: ApiRequestOptions) {
  const query = new URLSearchParams({ workspace_id: workspaceId });
  return fetchJson<GitHubActionPresets>(
    `/api/v1/github/action-presets?${query.toString()}`,
    options,
  );
}

export async function updateGitHubActionPresets(
  payload: UpdateGitHubActionPresetsRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<GitHubActionPresets>("/api/v1/github/action-presets", {
    ...options,
    init: { ...(options?.init ?? {}), method: "PUT", body: JSON.stringify(payload) },
  });
}

export async function resetGitHubActionPresets(workspaceId: string, options?: ApiRequestOptions) {
  const query = new URLSearchParams({ workspace_id: workspaceId });
  return fetchJson<GitHubActionPresets>(`/api/v1/github/action-presets/reset?${query.toString()}`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "POST" },
  });
}
