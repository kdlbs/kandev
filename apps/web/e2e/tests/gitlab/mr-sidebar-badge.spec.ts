import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { AppSidebarPage } from "../../pages/app-sidebar-page";
import { GITLAB_HOST, GITLAB_PROJECT } from "../../helpers/gitlab";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

let nextIID = 4400;

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
      title: overrides.title ?? `Sidebar badge fixture MR ${iid}`,
      url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${iid}`,
      web_url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${iid}`,
      state: overrides.state ?? "open",
      head_branch: `feature/sidebar-badge-${iid}`,
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
 * session's `on_turn_complete` moves the card out of the start column mid-
 * test, and the badge is a pure function of the task's linked MRs so an
 * agent turn adds nothing here.
 */
async function seedBoardTask(apiClient: ApiClient, seedData: SeedData, title: string) {
  return apiClient.createTask(seedData.workspaceId, title, {
    description: "Sidebar MR badge fixture task",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
}

test.describe("GitLab MR badge on the sidebar", () => {
  test("E1: a single linked open MR renders the badge with count=1 and state=open", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await ensureGitLabConfigured(apiClient, seedData);
    const iid = nextMRIID();
    await seedMR(apiClient, seedData.workspaceId, iid, { state: "open" });
    const task = await seedBoardTask(apiClient, seedData, "Sidebar single MR badge task");
    await linkMR(apiClient, seedData, task.id, iid);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCard(task.id)).toBeVisible({ timeout: 45_000 });

    const sidebar = new AppSidebarPage(testPage);
    await expect(sidebar.row(task.id)).toBeVisible({ timeout: 15_000 });
    const icon = sidebar.mrBadge(task.id);
    await expect(icon).toBeVisible({ timeout: 15_000 });
    await expect(icon).toHaveAttribute("data-mr-count", "1");
    await expect(icon).toHaveAttribute("data-mr-state", "open");
  });

  test("E2: a task with no linked MR renders no sidebar badge", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const task = await seedBoardTask(apiClient, seedData, "Sidebar no MR badge task");

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCard(task.id)).toBeVisible({ timeout: 45_000 });

    const sidebar = new AppSidebarPage(testPage);
    await expect(sidebar.row(task.id)).toBeVisible({ timeout: 15_000 });
    await expect(sidebar.mrBadge(task.id)).toHaveCount(0);
  });

  test("E3: a linked PR and MR both render on the sidebar row, PR before MR", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await ensureGitLabConfigured(apiClient, seedData);
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");

    const iid = nextMRIID();
    await seedMR(apiClient, seedData.workspaceId, iid, { state: "merged" });
    const task = await seedBoardTask(apiClient, seedData, "Sidebar PR and MR badge task");
    await linkMR(apiClient, seedData, task.id, iid);
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 901,
      pr_url: "https://github.com/testorg/testrepo/pull/901",
      pr_title: "Sidebar companion PR",
      head_branch: "feat/sidebar-companion",
      base_branch: "main",
      author_login: "test-user",
      state: "open",
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCard(task.id)).toBeVisible({ timeout: 45_000 });

    const sidebar = new AppSidebarPage(testPage);
    const row = sidebar.row(task.id);
    await expect(row).toBeVisible({ timeout: 15_000 });
    const prIcon = sidebar.prBadge(task.id);
    const mrIcon = sidebar.mrBadge(task.id);
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

    const spacing = await row.evaluate((el, titleText) => {
      const title = [...el.querySelectorAll("span")].find(
        (candidate) =>
          candidate.classList.contains("whitespace-nowrap") && candidate.textContent === titleText,
      );
      const pr = el.querySelector('[data-testid^="pr-task-icon-"]');
      if (!title || !pr) return { found: false, gap: -1, titleTop: -1, prTop: -1 };
      const titleBox = title.getBoundingClientRect();
      const prBox = pr.getBoundingClientRect();
      return {
        found: true,
        gap: prBox.left - titleBox.right,
        titleTop: titleBox.top,
        prTop: prBox.top,
      };
    }, "Sidebar PR and MR badge task");
    expect(spacing.found).toBe(true);
    expect(Math.abs(spacing.titleTop - spacing.prTop)).toBeLessThan(4);
    expect(spacing.gap).toBeGreaterThanOrEqual(0);
    expect(spacing.gap).toBeLessThan(32);
  });
});
