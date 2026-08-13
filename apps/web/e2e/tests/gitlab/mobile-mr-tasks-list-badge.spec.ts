import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { GITLAB_HOST, GITLAB_PROJECT } from "../../helpers/gitlab";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

const MR_IID = 4580;

async function seedTaskWithPRAndMR(apiClient: ApiClient, seedData: SeedData, title: string) {
  await apiClient.configureGitLab(seedData.workspaceId, GITLAB_HOST);
  await apiClient.updateRepository(seedData.repositoryId, {
    provider: "gitlab",
    provider_host: GITLAB_HOST,
    provider_owner: "platform",
    provider_name: "kandev",
  });
  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("test-user");

  const now = new Date().toISOString();
  await apiClient.mockGitLabAddMRs(seedData.workspaceId, GITLAB_PROJECT, [
    {
      iid: MR_IID,
      id: MR_IID + 10_000,
      project_id: 101,
      title: "Mobile tasks list badge fixture MR",
      url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${MR_IID}`,
      web_url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${MR_IID}`,
      state: "open",
      head_branch: "feature/mobile-tasks-list-badge",
      head_sha: "sha-mobile-tasks-list-badge",
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
  // auto-started session's `on_turn_complete` moves the task out from under
  // the assertion, which is what made the desktop badge specs flaky.
  const task = await apiClient.createTask(seedData.workspaceId, title, {
    description: "Mobile tasks list MR badge fixture task",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  await apiClient.linkTaskGitLabMR(seedData.workspaceId, {
    task_id: task.id,
    repository_id: seedData.repositoryId,
    mr_url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${MR_IID}`,
  });
  await apiClient.mockGitHubAssociateTaskPR({
    task_id: task.id,
    owner: "testorg",
    repo: "testrepo",
    pr_number: 904,
    pr_url: "https://github.com/testorg/testrepo/pull/904",
    pr_title: "Mobile tasks list companion PR",
    head_branch: "feat/mobile-tasks-list-companion",
    base_branch: "main",
    author_login: "test-user",
    state: "open",
  });
  return task;
}

test.describe("mobile GitLab MR badge on the /tasks list rows", () => {
  test("E7: renders the MR badge on a mobile viewport without causing horizontal overflow", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const task = await seedTaskWithPRAndMR(apiClient, seedData, "Mobile tasks list MR badge task");

    await testPage.goto("/tasks");
    const row = testPage.getByTestId("tasks-list-row").filter({ hasText: task.title });
    await expect(row).toBeVisible({ timeout: 45_000 });

    // Mobile has no `display-button`; the display menu lives behind the
    // "Open menu" drawer (see mobile-task-listing-display.spec.ts).
    await testPage.getByRole("button", { name: "Open menu" }).click();
    const menu = testPage.getByRole("dialog", { name: "Menu" });
    await menu.getByText("Show task details", { exact: true }).click();
    await testPage.keyboard.press("Escape");
    await expect(menu).toHaveCount(0);

    const icon = row.getByTestId(`mr-task-icon-${task.id}`);
    await expect(icon).toBeVisible({ timeout: 15_000 });

    await assertNoDocumentHorizontalOverflow(testPage, "mobile /tasks list with MR badge");
  });
});
