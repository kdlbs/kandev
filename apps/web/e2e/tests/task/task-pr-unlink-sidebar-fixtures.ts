import { expect } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

export const SIDEBAR_PR_UNLINK_TITLE = "Sidebar PR unlink target";
export const SIDEBAR_PR_UNLINK_LABEL = "Remove kandev-e2e/sidebar-pr-unlink #71 from task";

export async function seedSidebarTaskWithPR(
  apiClient: ApiClient,
  seedData: SeedData,
  navigationTitle: string,
) {
  const stepOptions = {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  };
  const navigationTask = await apiClient.seedTask(
    seedData.workspaceId,
    navigationTitle,
    stepOptions,
  );
  const targetTask = await apiClient.seedTask(seedData.workspaceId, SIDEBAR_PR_UNLINK_TITLE, {
    ...stepOptions,
    state: "IN_PROGRESS",
  });

  for (const taskId of [navigationTask.task_id, targetTask.task_id]) {
    await apiClient.seedTaskSession(taskId, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
    });
  }

  await apiClient.mockGitHubAssociateTaskPR({
    workspace_id: seedData.workspaceId,
    task_id: targetTask.task_id,
    repository_id: seedData.repositoryId,
    owner: "kandev-e2e",
    repo: "sidebar-pr-unlink",
    pr_number: 71,
    pr_url: "https://github.test/kandev-e2e/sidebar-pr-unlink/pull/71",
    pr_title: "Sidebar unlink fixture",
    head_branch: "feature/sidebar-unlink",
    base_branch: "main",
    author_login: "test-user",
    state: "open",
  });

  await expect.poll(() => apiClient.listTaskPRs(targetTask.task_id)).toHaveLength(1);
  return { navigationTask, targetTask };
}
