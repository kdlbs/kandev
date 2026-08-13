import { test, expect } from "../../fixtures/test-base";
import { GITLAB_HOST, GITLAB_PROJECT } from "../../helpers/gitlab";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

let nextIID = 4500;

function nextMRIID(): number {
  nextIID += 1;
  return nextIID;
}

async function seedMR(
  apiClient: ApiClient,
  workspaceId: string,
  iid: number,
  overrides: Partial<{
    state: string;
    draft: boolean;
    title: string;
  }> = {},
) {
  const now = new Date().toISOString();
  await apiClient.mockGitLabAddMRs(workspaceId, GITLAB_PROJECT, [
    {
      iid,
      id: iid + 10_000,
      project_id: 101,
      title: overrides.title ?? `Tasks list badge fixture MR ${iid}`,
      url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${iid}`,
      web_url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${iid}`,
      state: overrides.state ?? "open",
      head_branch: `feature/tasks-list-badge-${iid}`,
      head_sha: `sha-${iid}`,
      base_branch: "main",
      author_username: "contributor",
      project_namespace: "platform",
      project_path: GITLAB_PROJECT,
      body: "",
      draft: overrides.draft ?? false,
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
}

async function linkMR(
  apiClient: ApiClient,
  seedData: SeedData,
  taskId: string,
  iid: number,
): Promise<void> {
  await apiClient.linkTaskGitLabMR(seedData.workspaceId, {
    task_id: taskId,
    repository_id: seedData.repositoryId,
    mr_url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${iid}`,
  });
}

async function ensureGitLabConfigured(apiClient: ApiClient, seedData: SeedData): Promise<void> {
  await apiClient.configureGitLab(seedData.workspaceId, GITLAB_HOST);
  await apiClient.updateRepository(seedData.repositoryId, {
    provider: "gitlab",
    provider_host: GITLAB_HOST,
    provider_owner: "platform",
    provider_name: "kandev",
  });
}

/**
 * Seeds a board card with NO agent, deliberately — see
 * `mr-task-card-badge.spec.ts`'s `seedBoardTask` comment. An auto-started
 * session's `on_turn_complete` moves the task out from under the assertion,
 * and the badge is a pure function of the task's linked MRs so an agent turn
 * adds nothing here.
 */
async function seedBoardTask(apiClient: ApiClient, seedData: SeedData, title: string) {
  return apiClient.createTask(seedData.workspaceId, title, {
    description: "Tasks list MR badge fixture task",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
}

async function showTaskDetails(testPage: import("@playwright/test").Page): Promise<void> {
  await testPage.getByTestId("display-button").click();
  await testPage.getByText("Show task details", { exact: true }).click();
}

test.describe("GitLab MR badge on the /tasks list rows", () => {
  test("E4: the rich row shows the badge when a linked MR exists", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await ensureGitLabConfigured(apiClient, seedData);
    const iid = nextMRIID();
    await seedMR(apiClient, seedData.workspaceId, iid, { state: "open" });
    const task = await seedBoardTask(apiClient, seedData, "Tasks list single MR badge task");
    await linkMR(apiClient, seedData, task.id, iid);

    // Explicit, not assumed: tasks_list_show_details is a *bool PATCH field
    // on the backend, so an unset field in the testPage fixture's settings
    // reset leaves whatever a previous test in this worker persisted.
    await apiClient.saveUserSettings({ tasks_list_show_details: false });

    await testPage.goto("/tasks");
    const row = testPage.getByTestId("tasks-list-row").filter({ hasText: task.title });
    await expect(row).toBeVisible({ timeout: 45_000 });
    await showTaskDetails(testPage);

    const icon = row.getByTestId(`mr-task-icon-${task.id}`);
    await expect(icon).toBeVisible({ timeout: 15_000 });
    await expect(icon).toHaveAttribute("data-mr-count", "1");
    await expect(icon).toHaveAttribute("data-mr-state", "open");
  });

  test("E5: the compact row shows neither badge with task details off", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await ensureGitLabConfigured(apiClient, seedData);
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    const iid = nextMRIID();
    await seedMR(apiClient, seedData.workspaceId, iid, { state: "open" });
    const task = await seedBoardTask(apiClient, seedData, "Tasks list compact badge task");
    await linkMR(apiClient, seedData, task.id, iid);
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 902,
      pr_url: "https://github.com/testorg/testrepo/pull/902",
      pr_title: "Tasks list companion PR",
      head_branch: "feat/tasks-list-companion",
      base_branch: "main",
      author_login: "test-user",
      state: "open",
    });

    // Explicit, not assumed: tasks_list_show_details is a *bool PATCH field
    // on the backend, so an unset field in the testPage fixture's settings
    // reset leaves whatever a previous test in this worker persisted.
    await apiClient.saveUserSettings({ tasks_list_show_details: false });

    await testPage.goto("/tasks");
    const row = testPage.getByTestId("tasks-list-row").filter({ hasText: task.title });
    await expect(row).toBeVisible({ timeout: 45_000 });

    await expect(row.getByTestId(`pr-task-icon-${task.id}`)).toHaveCount(0);
    await expect(row.getByTestId(`mr-task-icon-${task.id}`)).toHaveCount(0);
  });

  test("E6: the rich row shows PR before MR", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(120_000);
    await ensureGitLabConfigured(apiClient, seedData);
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");

    const iid = nextMRIID();
    await seedMR(apiClient, seedData.workspaceId, iid, { state: "merged" });
    const task = await seedBoardTask(apiClient, seedData, "Tasks list PR and MR badge task");
    await linkMR(apiClient, seedData, task.id, iid);
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 903,
      pr_url: "https://github.com/testorg/testrepo/pull/903",
      pr_title: "Tasks list PR before MR",
      head_branch: "feat/tasks-list-pr-mr",
      base_branch: "main",
      author_login: "test-user",
      state: "open",
    });

    // Explicit, not assumed: tasks_list_show_details is a *bool PATCH field
    // on the backend, so an unset field in the testPage fixture's settings
    // reset leaves whatever a previous test in this worker persisted.
    await apiClient.saveUserSettings({ tasks_list_show_details: false });

    await testPage.goto("/tasks");
    const row = testPage.getByTestId("tasks-list-row").filter({ hasText: task.title });
    await expect(row).toBeVisible({ timeout: 45_000 });
    await showTaskDetails(testPage);

    const prIcon = row.getByTestId(`pr-task-icon-${task.id}`);
    const mrIcon = row.getByTestId(`mr-task-icon-${task.id}`);
    await expect(prIcon).toBeVisible({ timeout: 15_000 });
    await expect(mrIcon).toBeVisible({ timeout: 15_000 });

    const order = await row.evaluate((el) => {
      const pr = el.querySelector('[data-testid^="pr-task-icon-"]');
      const mr = el.querySelector('[data-testid^="mr-task-icon-"]');
      if (!pr || !mr) return "missing";
      return pr.compareDocumentPosition(mr) & Node.DOCUMENT_POSITION_FOLLOWING
        ? "pr-then-mr"
        : "mr-then-pr";
    });
    expect(order).toBe("pr-then-mr");
  });
});
