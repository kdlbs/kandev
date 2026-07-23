import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";

export const REVIEW_OWNER = "testorg";
export const REVIEW_REPO = "testrepo";
export const REVIEW_SHARED_FILE = "shared-pr.ts";

export const REVIEW_PRS = [
  {
    number: 121,
    title: "First review branch",
    branch: "feat/first-review",
    marker: "FIRST_PR_MARKER",
    repositoryName: "E2E-Repo",
  },
  {
    number: 122,
    title: "Second review branch",
    branch: "feat/second-review",
    marker: "SECOND_PR_MARKER",
    repositoryName: "E2E-Repo-feat-second-review",
  },
] as const;

export async function seedMultiPRReviewTask(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  description = "/e2e:simple-message",
) {
  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("reviewer");

  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repositories: REVIEW_PRS.map((pr) => ({
        repository_id: seedData.repositoryId,
        base_branch: "main",
        checkout_branch: pr.branch,
      })),
      executor_profile_id: seedData.worktreeExecutorProfileId,
    },
  );

  await apiClient.mockGitHubAddPRs(
    REVIEW_PRS.map((pr) => ({
      number: pr.number,
      title: pr.title,
      state: "open",
      head_branch: pr.branch,
      base_branch: "main",
      author_login: "reviewer",
      repo_owner: REVIEW_OWNER,
      repo_name: REVIEW_REPO,
      html_url: `https://github.com/${REVIEW_OWNER}/${REVIEW_REPO}/pull/${pr.number}`,
      additions: 1,
      deletions: 0,
    })),
  );

  for (const pr of REVIEW_PRS) {
    await apiClient.mockGitHubAddPRFiles(REVIEW_OWNER, REVIEW_REPO, pr.number, [
      {
        filename: REVIEW_SHARED_FILE,
        status: "added",
        additions: 1,
        deletions: 0,
        patch: `@@ -0,0 +1 @@\n+${pr.marker}`,
      },
    ]);
    await apiClient.associateGitHubTaskPR({
      task_id: task.id,
      repository_id: seedData.repositoryId,
      pr_url: `https://github.com/${REVIEW_OWNER}/${REVIEW_REPO}/pull/${pr.number}`,
    });
  }

  return task;
}
