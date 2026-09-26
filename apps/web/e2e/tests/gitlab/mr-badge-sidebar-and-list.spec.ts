import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { GITLAB_HOST, GITLAB_PROJECT } from "../../helpers/gitlab";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

/**
 * The AppSidebar (app/layout.tsx) never unmounts across a client-side route
 * change, and this branch gave it both its own MR badges and its own
 * `useWorkspaceMRs` call. `useWorkspaceMRs(null)` resets the whole
 * `taskMRs.byWorkspaceId` map — not just one workspace — and the hook's
 * per-instance `fetchedRef` then stops the sidebar refilling what was deleted.
 *
 * So a second consumer that merely does not *want* MRs can blank the sidebar's
 * badges. `/tasks` was exactly that consumer: it gated its call on
 * `tasksListShowDetails`, which is off by default.
 *
 * A full page load hides this — both consumers mount in one commit, the reset
 * runs synchronously before the sidebar's fetch resolves, and the late response
 * repopulates. Only a client-side navigation, with the sidebar already hydrated,
 * exposes it. Hence the command palette below rather than `page.goto`.
 */

let nextIID = 700;

function nextMRIID(): number {
  nextIID += 1;
  return nextIID;
}

async function seedAndLinkMR(apiClient: ApiClient, seedData: SeedData, taskId: string) {
  const iid = nextMRIID();
  const now = new Date().toISOString();
  await apiClient.mockGitLabAddMRs(seedData.workspaceId, GITLAB_PROJECT, [
    {
      iid,
      id: iid + 10_000,
      project_id: 101,
      title: `Sidebar cache fixture MR ${iid}`,
      url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${iid}`,
      web_url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${iid}`,
      state: "open",
      head_branch: `feature/cache-${iid}`,
      head_sha: `sha-${iid}`,
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
  await apiClient.linkTaskGitLabMR(seedData.workspaceId, {
    task_id: taskId,
    repository_id: seedData.repositoryId,
    mr_url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${iid}`,
  });
  return iid;
}

async function configureGitLab(apiClient: ApiClient, seedData: SeedData) {
  await apiClient.configureGitLab(seedData.workspaceId, GITLAB_HOST);
  await apiClient.updateRepository(seedData.repositoryId, {
    provider: "gitlab",
    provider_host: GITLAB_HOST,
    provider_owner: "platform",
    provider_name: "kandev",
  });
}

// No agent: an auto-started session's on_turn_complete would move the task
// mid-test (see mr-task-card-badge.spec.ts for the full rationale).
async function seedTask(apiClient: ApiClient, seedData: SeedData, title: string) {
  return apiClient.createTask(seedData.workspaceId, title, {
    description: "MR badge fixture task",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
}

test.describe("GitLab MR badge on the sidebar and the /tasks list", () => {
  test("the sidebar's MR badge survives navigating into /tasks with task details off", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    // tasksListShowDetails is deliberately left at its default (off) — that is
    // the configuration the bug needed, so setting it here would test nothing.
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      enable_preview_on_click: false,
    });
    await configureGitLab(apiClient, seedData);

    const task = await seedTask(apiClient, seedData, "Sidebar MR cache task");
    await seedAndLinkMR(apiClient, seedData, task.id);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCard(task.id)).toBeVisible({ timeout: 45_000 });

    // The sidebar row's badge, not the board card's — the board has its own
    // useWorkspaceMRs via kanban-board.tsx and would recover on its own.
    const sidebarBadge = testPage.getByTestId("app-sidebar").getByTestId(`mr-task-icon-${task.id}`);
    await expect(sidebarBadge).toBeVisible({ timeout: 30_000 });

    // Client-side navigation through the app's own command, so the sidebar
    // instance (and its already-satisfied fetchedRef) stays mounted.
    await testPage.keyboard.press("ControlOrMeta+k");
    await testPage.getByRole("option", { name: "Switch to List View" }).click();
    await expect(testPage).toHaveURL(/\/tasks/);
    await expect(testPage.getByTestId("tasks-list")).toBeVisible({ timeout: 30_000 });

    // The regression: TasksPageClient mounting must not empty the slice the
    // sidebar is still rendering from. Given a few seconds so a wipe that is
    // merely slow still fails the assertion rather than racing past it.
    await expect(sidebarBadge).toBeVisible({ timeout: 10_000 });
    await expect(sidebarBadge).toHaveAttribute("data-mr-count", "1");
  });

  test("AC11: the /tasks rich list row renders the MR badge and its structured hover", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    // The badge only renders on this row in the rich ("show details") mode —
    // PrimaryTaskLine is given showContributions={false} otherwise.
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      enable_preview_on_click: false,
      tasks_list_show_details: true,
    });
    await configureGitLab(apiClient, seedData);

    const task = await seedTask(apiClient, seedData, "Tasks list MR badge task");
    const iid = await seedAndLinkMR(apiClient, seedData, task.id);

    await testPage.goto("/tasks");
    await testPage.waitForLoadState("networkidle");

    const row = testPage
      .getByTestId("tasks-list")
      .getByTestId("tasks-list-row")
      .filter({ hasText: "Tasks list MR badge task" })
      .first();
    await expect(row).toBeVisible({ timeout: 45_000 });

    // AC11: the badge reaches this surface at all. It needs the page's own
    // useWorkspaceMRs to have hydrated the slice, so this also guards the
    // fetch that the sidebar-cache fix above un-gated.
    const badge = row.getByTestId(`mr-task-icon-${task.id}`);
    await expect(badge).toBeVisible({ timeout: 30_000 });
    await expect(badge).toHaveAttribute("data-mr-count", "1");

    // AC1/AC24: the same structured summary the board card gets, not a
    // pipe-delimited string.
    await badge.hover();
    const summary = testPage
      .locator(
        '[data-slot="tooltip-content"]:not([data-state="closed"]) > [data-testid="mr-task-status-summary"]',
      )
      .first();
    await expect(summary).toBeVisible();
    await expect(summary.getByTestId("mr-task-status-iid")).toHaveText(`MR !${iid}`);
    await expect(summary).not.toContainText("|");
  });
});
