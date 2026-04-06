import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

/**
 * Regression test: navigating from a task with a worktree branch to a new
 * sessionless task must not display the previous task's branch in the topbar.
 *
 * Root cause: useSessionResumption kept stale local state (worktreeBranch)
 * from the previous session. On navigation, a race between async
 * checkAndResume calls for the old and new sessions could leave the old
 * branch value persisted in the topbar.
 */
test.describe("Stale topbar branch", () => {
  test("navigating from a task with a branch to a sessionless task does not show stale branch", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    // The branch pill is hidden below 1352px container width (@max-[1352px]/topbar:hidden).
    await testPage.setViewportSize({ width: 1600, height: 900 });

    // 1. Create Task A with worktree executor so it gets a real worktree branch
    const taskA = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Task With Branch",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );

    // 2. Wait for Task A's session to finish and capture its worktree branch
    let taskABranch: string | null = null;
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(taskA.id);
          const s = sessions[0];
          if (s?.worktree_branch) taskABranch = s.worktree_branch;
          return DONE_STATES.includes(s?.state);
        },
        { timeout: 30_000, message: "Waiting for Task A session to finish" },
      )
      .toBe(true);

    expect(taskABranch).toBeTruthy();

    // 3. Create Task B (no session, no repository)
    await apiClient.createTask(seedData.workspaceId, "Sessionless Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    // 4. Navigate to Task A to populate the store with its session/branch data
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const cardA = kanban.taskCardByTitle("Task With Branch");
    await expect(cardA).toBeVisible({ timeout: 10_000 });
    await cardA.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // Confirm topbar shows Task A's worktree branch
    const branchPill = testPage.getByTestId("topbar-branch-name");
    await expect(branchPill).toBeVisible({ timeout: 15_000 });
    await expect(branchPill).toHaveText(taskABranch!, { timeout: 5_000 });

    // 5. Navigate to Task B via the topbar home breadcrumb (SPA link) then clicking Task B.
    //    This preserves Zustand store state (activeSessionId from Task A persists).
    const homeLink = testPage.locator("header a[href='/']").first();
    await homeLink.click();
    await expect(kanban.board).toBeVisible({ timeout: 10_000 });

    const cardB = kanban.taskCardByTitle("Sessionless Task");
    await expect(cardB).toBeVisible({ timeout: 10_000 });
    await cardB.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    // 6. Verify Task B's page shows the correct title (breadcrumb in topbar)
    await expect(testPage.getByRole("link", { name: "Sessionless Task" })).toBeVisible({
      timeout: 10_000,
    });

    // 7. Task A's worktree branch must NOT appear on Task B's topbar.
    //    Wait for any async state updates to settle.
    await testPage.waitForTimeout(3_000);

    const branchAfterNav = testPage.getByTestId("topbar-branch-name");
    const isBranchVisible = await branchAfterNav.isVisible().catch(() => false);
    if (isBranchVisible) {
      const currentBranch = await branchAfterNav.textContent();
      expect(currentBranch).not.toBe(taskABranch);
    }
    // If no branch is visible, that's correct for a sessionless task
  });
});
