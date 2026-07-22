import type { GitHubPRStatus, MergeMethod, PRFeedback, RepoMergeMethods } from "@/lib/types/github";
import { fetchJson, type ApiRequestOptions } from "../client";

export type PRStatusRef = { owner: string; repo: string; number: number };
export type PRReviewEvent = "APPROVE" | "COMMENT" | "REQUEST_CHANGES";

function workspaceQuery(workspaceId: string) {
  return new URLSearchParams({ workspace_id: workspaceId });
}

export async function getPRFeedback(
  workspaceId: string,
  owner: string,
  repo: string,
  number: number,
  options?: ApiRequestOptions,
) {
  return fetchJson<PRFeedback>(
    `/api/v1/github/prs/${owner}/${repo}/${number}?${workspaceQuery(workspaceId)}`,
    options,
  );
}

export async function getPRStatus(
  workspaceId: string,
  owner: string,
  repo: string,
  number: number,
  options?: ApiRequestOptions,
) {
  return fetchJson<GitHubPRStatus>(
    `/api/v1/github/prs/${owner}/${repo}/${number}/status?${workspaceQuery(workspaceId)}`,
    options,
  );
}

export async function getPRStatusesBatch(
  workspaceId: string,
  refs: PRStatusRef[],
  options?: ApiRequestOptions,
) {
  return fetchJson<{ statuses: Record<string, GitHubPRStatus> }>(`/api/v1/github/prs/statuses`, {
    ...options,
    init: {
      method: "POST",
      body: JSON.stringify({ workspace_id: workspaceId, refs }),
      ...(options?.init ?? {}),
    },
  });
}

export async function submitPRReview(
  workspaceId: string,
  ref: PRStatusRef,
  event: PRReviewEvent,
  body?: string,
) {
  return fetchJson<{ submitted: boolean }>(
    `/api/v1/github/prs/${ref.owner}/${ref.repo}/${ref.number}/reviews?${workspaceQuery(workspaceId)}`,
    {
      init: {
        method: "POST",
        body: JSON.stringify({ event, body: body ?? "" }),
      },
    },
  );
}

export async function mergePR(
  workspaceId: string,
  owner: string,
  repo: string,
  number: number,
  mergeMethod?: MergeMethod,
) {
  return fetchJson<{ merged: boolean }>(
    `/api/v1/github/prs/${owner}/${repo}/${number}/merge?${workspaceQuery(workspaceId)}`,
    {
      init: {
        method: "PUT",
        body: JSON.stringify({ merge_method: mergeMethod ?? "" }),
      },
    },
  );
}

export async function getRepoMergeMethods(
  workspaceId: string,
  owner: string,
  repo: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<RepoMergeMethods>(
    `/api/v1/github/repos/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/merge-methods?${workspaceQuery(workspaceId)}`,
    options,
  );
}
