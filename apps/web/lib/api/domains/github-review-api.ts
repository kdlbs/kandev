import { fetchJson } from "../client";

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

export async function requestPRReviewers(
  owner: string,
  repo: string,
  number: number,
  reviewers: string[],
  workspaceId: string,
) {
  return fetchJson<{ requested: boolean }>(
    `/api/v1/github/prs/${owner}/${repo}/${number}/requested-reviewers?workspace_id=${encodeURIComponent(workspaceId)}`,
    {
      init: {
        method: "POST",
        body: JSON.stringify({ reviewers }),
      },
    },
  );
}
