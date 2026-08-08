import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { GITLAB_HOST, GITLAB_PROJECT } from "../../helpers/gitlab";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

let nextIID = 400;

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
      title: overrides.title ?? `Badge fixture MR ${iid}`,
      url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${iid}`,
      web_url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${iid}`,
      state: overrides.state ?? "open",
      head_branch: `feature/badge-${iid}`,
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
 * Seeds a board card with NO agent, deliberately.
 *
 * `createTaskWithAgent` auto-starts a session on the start step (the Kanban
 * template's `on_enter: auto_start_agent`), and that same step carries
 * `on_turn_complete: move_to_step review` — so the card leaves the start
 * column the instant the mock agent's turn ends. Every assertion here was
 * therefore racing that transition: on a loaded CI runner the agent won and
 * the card was already in Review, and when it moved mid-assertion the badge
 * detached from under `.hover()` and the test burned its full timeout.
 *
 * The badge is a pure function of the task's linked MRs, so an agent turn
 * adds nothing to these ACs. Without one, no turn ever completes and the
 * card stays put.
 */
async function seedBoardTask(apiClient: ApiClient, seedData: SeedData, title: string) {
  return apiClient.createTask(seedData.workspaceId, title, {
    description: "MR badge fixture task",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
}

test.describe("GitLab MR badge on the Kanban card", () => {
  test("AC27: a single linked MR renders the badge with count=1 and the MR's state", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await ensureGitLabConfigured(apiClient, seedData);
    const iid = nextMRIID();
    await seedMR(apiClient, seedData.workspaceId, iid, { state: "open" });
    const task = await seedBoardTask(apiClient, seedData, "Single MR badge task");
    await linkMR(apiClient, seedData, task.id, iid);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    // Located by task id rather than by (title, column): which column a card
    // sits in is irrelevant to the badge, and coupling to it makes the
    // assertion race any workflow transition (see seedBoardTask).
    await expect(kanban.taskCard(task.id)).toBeVisible({ timeout: 45_000 });

    const icon = kanban.board.getByTestId(`mr-task-icon-${task.id}`);
    await expect(icon).toBeVisible({ timeout: 15_000 });
    await expect(icon).toHaveAttribute("data-mr-count", "1");
    await expect(icon).toHaveAttribute("data-mr-state", "open");
  });

  test("AC29: a task with no linked MR renders no badge", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const task = await seedBoardTask(apiClient, seedData, "No MR badge task");

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCard(task.id)).toBeVisible({ timeout: 45_000 });

    await expect(kanban.board.getByTestId(`mr-task-icon-${task.id}`)).toHaveCount(0);
  });

  test("AC28: 2+ linked MRs render one badge whose colour is the worst open MR's", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await ensureGitLabConfigured(apiClient, seedData);
    const mergedIID = nextMRIID();
    const failingIID = nextMRIID();
    await seedMR(apiClient, seedData.workspaceId, mergedIID, { state: "merged" });
    await seedMR(apiClient, seedData.workspaceId, failingIID, { state: "open" });
    // The open MR's pipeline is failing (red) — must win over the merged
    // (purple) MR per AC28's "terminal MRs dropped when one is open" rule.
    // Seeded before linking: SyncTaskMR fetches status at link time, and the
    // mock's pipeline list is shared per-project (not per-MR).
    await apiClient.mockGitLabAddPipelines(seedData.workspaceId, GITLAB_PROJECT, [
      {
        id: failingIID + 50_000,
        iid: 1,
        status: "failed",
        source: "push",
        ref: `feature/badge-${failingIID}`,
        sha: `sha-${failingIID}`,
        web_url: "",
        jobs_total: 2,
        jobs_passing: 0,
      },
    ]);
    const task = await seedBoardTask(apiClient, seedData, "Multi MR badge task");
    await linkMR(apiClient, seedData, task.id, mergedIID);
    await linkMR(apiClient, seedData, task.id, failingIID);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCard(task.id)).toBeVisible({ timeout: 45_000 });

    const icon = kanban.board.getByTestId(`mr-task-icon-${task.id}`);
    await expect(icon).toBeVisible({ timeout: 15_000 });
    await expect
      .poll(async () => icon.getAttribute("data-mr-count"), { timeout: 15_000 })
      .toBe("2");
    await expect(icon).toHaveClass(/text-red-500/);
  });

  test("AC30/AC37: a linked PR and MR both render, PR before MR, and the badge tooltip names a non-open state", async ({
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
    const task = await seedBoardTask(apiClient, seedData, "PR and MR badge task");
    await linkMR(apiClient, seedData, task.id, iid);
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 900,
      pr_url: "https://github.com/testorg/testrepo/pull/900",
      pr_title: "Companion PR",
      head_branch: "feat/companion",
      base_branch: "main",
      author_login: "test-user",
      state: "open",
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const card = kanban.taskCard(task.id);
    await expect(card).toBeVisible({ timeout: 45_000 });
    const prIcon = card.getByTestId(`pr-task-icon-${task.id}`);
    const mrIcon = card.getByTestId(`mr-task-icon-${task.id}`);
    await expect(prIcon).toBeVisible({ timeout: 15_000 });
    await expect(mrIcon).toBeVisible({ timeout: 15_000 });

    const order = await card.evaluate((el) => {
      const pr = el.querySelector('[data-testid^="pr-task-icon-"]');
      const mr = el.querySelector('[data-testid^="mr-task-icon-"]');
      if (!pr || !mr) return "missing";
      return pr.compareDocumentPosition(mr) & Node.DOCUMENT_POSITION_FOLLOWING
        ? "pr-then-mr"
        : "mr-then-pr";
    });
    expect(order).toBe("pr-then-mr");

    // AC37: state is never colour-only — the tooltip names it explicitly.
    await mrIcon.hover();
    await expect(testPage.getByRole("tooltip")).toContainText("merged");
  });
});
