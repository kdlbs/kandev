import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  GitHubStatusResponse,
  GitHubOrg,
  GitHubRepoInfo,
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
} from "@/lib/types/github";

// Status
export async function fetchGitHubStatus(options?: ApiRequestOptions) {
  return fetchJson<GitHubStatusResponse>("/api/v1/github/status", options);
}

// Task PR associations
export async function listTaskPRs(taskIds: string[], options?: ApiRequestOptions) {
  const query = new URLSearchParams();
  query.set("task_ids", taskIds.join(","));
  return fetchJson<TaskPRsResponse>(`/api/v1/github/task-prs?${query.toString()}`, options);
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
export async function listReviewWatches(workspaceId: string, options?: ApiRequestOptions) {
  const query = new URLSearchParams({ workspace_id: workspaceId });
  return fetchJson<ReviewWatchesResponse>(
    `/api/v1/github/watches/review?${query.toString()}`,
    options,
  );
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
