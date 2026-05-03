import { fetchJson, type ApiRequestOptions } from "../client";
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
  SearchPRsResponse,
  SearchIssuesResponse,
  GitHubPRStatus,
  GitHubActionPresets,
  UpdateGitHubActionPresetsRequest,
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
  payload: UpdateReviewWatchRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<ReviewWatch>(`/api/v1/github/watches/review/${id}`, {
    ...options,
    init: { method: "PUT", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function deleteReviewWatch(id: string, options?: ApiRequestOptions) {
  return fetchJson<{ success: boolean }>(`/api/v1/github/watches/review/${id}`, {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}

export async function triggerReviewWatch(id: string, options?: ApiRequestOptions) {
  return fetchJson<TriggerReviewResponse>(`/api/v1/github/watches/review/${id}/trigger`, {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
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
  payload: UpdateIssueWatchRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<IssueWatch>(`/api/v1/github/watches/issue/${id}`, {
    ...options,
    init: { method: "PUT", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function deleteIssueWatch(id: string, options?: ApiRequestOptions) {
  return fetchJson<{ deleted: boolean }>(`/api/v1/github/watches/issue/${id}`, {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}

export async function triggerIssueWatch(id: string, options?: ApiRequestOptions) {
  return fetchJson<TriggerIssueResponse>(`/api/v1/github/watches/issue/${id}/trigger`, {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
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

// User PR / issue search (for the /github page).
// Pass `query` to use a verbatim GitHub search string, or `filter` to append to
// the default (type:pr state:open / type:issue state:open).
type SearchParams = {
  query?: string;
  filter?: string;
  page?: number;
  perPage?: number;
};

function buildSearchQuery(params: SearchParams) {
  const search = new URLSearchParams();
  if (params.query) search.set("query", params.query);
  if (params.filter) search.set("filter", params.filter);
  if (params.page && params.page > 1) search.set("page", String(params.page));
  if (params.perPage) search.set("per_page", String(params.perPage));
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
