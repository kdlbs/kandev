import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { GITLAB_HOST, GITLAB_PROJECT } from "../../helpers/gitlab";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

const MR_IID = 480;

async function seedBoardTaskWithMR(apiClient: ApiClient, seedData: SeedData, title: string) {
  await apiClient.configureGitLab(seedData.workspaceId, GITLAB_HOST);
  await apiClient.updateRepository(seedData.repositoryId, {
    provider: "gitlab",
    provider_host: GITLAB_HOST,
    provider_owner: "platform",
    provider_name: "kandev",
  });
  const now = new Date().toISOString();
  await apiClient.mockGitLabAddMRs(seedData.workspaceId, GITLAB_PROJECT, [
    {
      iid: MR_IID,
      id: MR_IID + 10_000,
      project_id: 101,
      title: "Mobile badge fixture MR",
      url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${MR_IID}`,
      web_url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${MR_IID}`,
      state: "open",
      head_branch: "feature/mobile-badge",
      head_sha: "sha-mobile-badge",
      base_branch: "main",
      author_username: "contributor",
      project_namespace: "platform",
      project_path: GITLAB_PROJECT,
      body: "",
      draft: false,
      merge_status: "can_be_merged",
      has_conflicts: false,
      additions: 1,
      deletions: 1,
      reviewers: [],
      assignees: [],
      created_at: now,
      updated_at: now,
    },
  ]);
  // No agent, deliberately — see the desktop spec's seedBoardTask. An
  // auto-started session's `on_turn_complete` moves the card out of the start
  // column mid-test, which is what made the desktop badge specs flaky.
  const task = await apiClient.createTask(seedData.workspaceId, title, {
    description: "MR badge fixture task",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  await apiClient.linkTaskGitLabMR(seedData.workspaceId, {
    task_id: task.id,
    repository_id: seedData.repositoryId,
    mr_url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${MR_IID}`,
  });
  return task;
}

test.describe("mobile GitLab MR badge on the Kanban card", () => {
  test("renders the same badge on a mobile viewport without causing horizontal overflow", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const task = await seedBoardTaskWithMR(apiClient, seedData, "Mobile MR badge task");

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCard(task.id)).toBeVisible({ timeout: 45_000 });

    const icon = kanban.board.getByTestId(`mr-task-icon-${task.id}`);
    await expect(icon).toBeVisible({ timeout: 15_000 });
    await expect(icon).toHaveAttribute("data-mr-count", "1");
    await expect(icon).toHaveAttribute("data-mr-state", "open");

    await assertNoDocumentHorizontalOverflow(testPage, "mobile Kanban board with MR badge");
  });
});
