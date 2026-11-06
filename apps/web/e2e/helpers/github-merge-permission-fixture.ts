import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";

export const MERGE_ACCESS_OWNER = "upstream-team";
export const MERGE_ACCESS_REPO = "shared-project";
export const MERGE_ACCESS_PR = 917;
export const MERGE_ACCESS_ACTOR = "contributor";
export const MERGE_ACCESS_DENIED = `${MERGE_ACCESS_ACTOR} cannot merge into ${MERGE_ACCESS_OWNER}/${MERGE_ACCESS_REPO}.`;

export async function seedMergePermissionTask(
  api: ApiClient,
  seed: SeedData,
  pushAccess: boolean | null,
) {
  await api.mockGitHubReset();
  await api.mockGitHubSetUser(MERGE_ACCESS_ACTOR);
  const task = await api.createTaskWithAgent(
    seed.workspaceId,
    "Review an upstream contribution",
    seed.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
    },
  );
  await api.mockGitHubSetRepositoryDetails(MERGE_ACCESS_ACTOR, MERGE_ACCESS_REPO, true, true);
  await api.mockGitHubSetRepositoryDetails(MERGE_ACCESS_OWNER, MERGE_ACCESS_REPO, pushAccess);
  await api.mockGitHubAssociateTaskPR({
    task_id: task.id,
    owner: MERGE_ACCESS_OWNER,
    repo: MERGE_ACCESS_REPO,
    pr_number: MERGE_ACCESS_PR,
    pr_url: `https://github.com/${MERGE_ACCESS_OWNER}/${MERGE_ACCESS_REPO}/pull/${MERGE_ACCESS_PR}`,
    pr_title: "Complete prompts handled by input extensions",
    head_branch: "fix/input-extensions",
    base_branch: "main",
    author_login: MERGE_ACCESS_ACTOR,
    state: "open",
    review_state: "approved",
    checks_state: "success",
    mergeable_state: "clean",
    review_count: 1,
    required_reviews: 1,
    pending_review_count: 0,
    checks_total: 1,
    checks_passing: 1,
  });
  await api.mockGitHubSeedPRFeedback({
    owner: MERGE_ACCESS_OWNER,
    repo: MERGE_ACCESS_REPO,
    pr_number: MERGE_ACCESS_PR,
    checks: [{ name: "Tests", status: "completed", conclusion: "success" }],
    reviews: [
      { id: 1, author: "maintainer", state: "APPROVED", created_at: "2026-10-15T10:00:00Z" },
    ],
  });
  return task;
}
