import type { ApiClient } from "./api-client";

export type MockPRFeedbackSeed = Parameters<ApiClient["mockGitHubSeedPRFeedback"]>[0];
export type MockCheckRunSeed = NonNullable<MockPRFeedbackSeed["checks"]>[number];
export type MockWorkflowRunSeed = NonNullable<MockPRFeedbackSeed["workflow_runs"]>[number];

export type PRCheckIdentity = {
  number: number;
  headSHA: string;
  headBranch: string;
  headRepoOwner: string;
  headRepoName: string;
};

export function makePRCheckRun({
  id,
  suiteId,
  name,
  status,
  conclusion,
  htmlUrl,
}: {
  id: number;
  suiteId: number;
  name: string;
  status: string;
  conclusion?: string;
  htmlUrl: string;
}): MockCheckRunSeed {
  return {
    id,
    app_id: 15368,
    app_slug: "github-actions",
    check_suite_id: suiteId,
    name,
    source: "check_run",
    status,
    conclusion,
    html_url: htmlUrl,
    output: "",
    started_at: status === "in_progress" ? "2026-09-30T14:32:41Z" : null,
    completed_at: status === "completed" ? "2026-09-30T14:39:15Z" : null,
  };
}

export function makePRWorkflowRun({
  id,
  suiteId,
  identity,
  name = "Preview",
  event = "pull_request_target",
  status,
  conclusion,
  url,
}: {
  id: number;
  suiteId: number;
  identity: PRCheckIdentity;
  name?: string;
  event?: string;
  status: string;
  conclusion?: string | null;
  url: string;
}): MockWorkflowRunSeed {
  return {
    id,
    check_suite_id: suiteId,
    run_attempt: 1,
    workflow_id: 266562986,
    name,
    event,
    status,
    conclusion,
    head_sha: identity.headSHA,
    head_branch: identity.headBranch,
    head_repo_owner: identity.headRepoOwner,
    head_repo_name: identity.headRepoName,
    html_url: url,
    created_at: "2026-09-30T14:32:41Z",
    updated_at: status === "completed" ? "2026-09-30T14:39:15Z" : "2026-09-30T14:32:41Z",
    pull_requests: [
      {
        number: identity.number,
        head_sha: identity.headSHA,
        head_branch: identity.headBranch,
        head_repo_owner: identity.headRepoOwner,
        head_repo_name: identity.headRepoName,
      },
    ],
  };
}
