import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";

export const ROOT_TEXT = "Please correct the token exchange.";
export const REPLY_TEXT = "The automated check found the same issue.";

export async function seedReviewThread(apiClient: ApiClient, seedData: SeedData) {
  const owner = "testorg";
  const repo = "testrepo";
  const prNumber = 821;
  const url = `https://github.com/${owner}/${repo}/pull/${prNumber}`;
  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("test-user");
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Review mixed thread context",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await apiClient.mockGitHubAssociateTaskPR({
    task_id: task.id,
    owner,
    repo,
    pr_number: prNumber,
    pr_url: url,
    pr_title: "Correct token exchange",
    head_branch: "fix/token",
    base_branch: "main",
    author_login: "test-user",
    state: "open",
  });
  await apiClient.mockGitHubSeedPRFeedback({
    owner,
    repo,
    pr_number: prNumber,
    comments: [
      {
        id: 8211,
        author: "reviewer",
        body: ROOT_TEXT,
        path: "auth.ts",
        line: 7,
        side: "RIGHT",
        comment_type: "review",
        created_at: "2026-08-01T09:00:00Z",
      },
      {
        id: 8212,
        author: "review-bot",
        author_is_bot: true,
        body: REPLY_TEXT,
        in_reply_to: 8211,
        comment_type: "review",
        created_at: "2026-08-01T09:01:00Z",
      },
    ],
  });
  return task;
}
