import type { MockGitLabIssueSeed } from "../../helpers/api-client";
import { GITLAB_PROJECT } from "../../helpers/gitlab";

export const ISSUES_ENDPOINT = /^\/api\/v1\/gitlab\/user\/issues$/;

export function seededIssue(
  iid: number,
  title: string,
  overrides: { milestone?: string } = {},
): MockGitLabIssueSeed {
  const now = new Date().toISOString();
  return {
    id: iid + 20_000,
    iid,
    project_id: 101,
    title,
    body: `Body for ${title}`,
    url: `https://gitlab.example.test/${GITLAB_PROJECT}/-/issues/${iid}`,
    web_url: `https://gitlab.example.test/${GITLAB_PROJECT}/-/issues/${iid}`,
    state: "opened",
    author_username: "reporter",
    project_namespace: "platform",
    project_path: GITLAB_PROJECT,
    labels: [],
    assignees: [],
    milestone: overrides.milestone ?? "",
    created_at: now,
    updated_at: now,
  };
}
