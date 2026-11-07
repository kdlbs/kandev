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
  context: string | { workspaceId: string; expectedHost?: string },
) {
  const { workspaceId, expectedHost } =
    typeof context === "string" ? { workspaceId: context } : context;
  const query = new URLSearchParams({ workspace_id: workspaceId });
  if (expectedHost) query.set("expected_host", expectedHost);
  return fetchJson<{ requested: boolean }>(
    `/api/v1/github/prs/${owner}/${repo}/${number}/requested-reviewers?${query}`,
    {
      init: {
        method: "POST",
        body: JSON.stringify({ reviewers }),
      },
    },
  );
}
