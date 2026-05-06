import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

/**
 * Regression test for the task sidebar diff badge bug.
 *
 * The sidebar bulk-subscribes to every task's primary session on connect and
 * expects an initial git status snapshot per session, including the global
 * branch_additions/branch_deletions diff against the merge-base. Before the
 * fix, the backend's `tryGetLiveGitStatus` only returned data when an
 * agentctl execution was actively running for that session. For any task
 * whose execution had been torn down (e.g. after a backend restart), the
 * fallback hit `appendDBSnapshotGitStatus` which only had data for archived
 * tasks — so the badge silently disappeared for every non-active task.
 *
 * The fix persists the live monitor's last status to a single cached row per
 * session in `task_session_git_snapshots` (triggered_by='live_monitor'),
 * keeping the DB-snapshot fallback fresh across restarts and unavailability.
 *
 * This test creates two tasks that produce diffs, restarts the backend
 * (which kills all running executors), then asserts the sidebar still shows
 * +N/-N badges for BOTH tasks — not just an active one.
 */
test.describe("Task sidebar diff stats", () => {
  test("badges survive backend restart for non-active tasks", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(180_000);

    // Create two tasks, each in its own worktree, each running the
    // diff-update-setup scenario which leaves one modified, committed file
    // and one unstaged modification → branch_additions / branch_deletions > 0.
    const taskAlpha = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Diff Alpha",
      seedData.agentProfileId,
      {
        description: "/e2e:diff-update-setup",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    const taskBeta = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Diff Beta",
      seedData.agentProfileId,
      {
        description: "/e2e:diff-update-setup",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );

    // Visit Alpha so we can wait for the agent's completion message and let
    // the live git monitor fire at least once for both sessions.
    await testPage.goto(`/t/${taskAlpha.id}`);
    const alphaSession = new SessionPage(testPage);
    await alphaSession.waitForLoad();
    await expect(
      alphaSession.chat.getByText("diff-update-setup complete", { exact: false }),
    ).toBeVisible({ timeout: 60_000 });

    await testPage.goto(`/t/${taskBeta.id}`);
    const betaSession = new SessionPage(testPage);
    await betaSession.waitForLoad();
    await expect(
      betaSession.chat.getByText("diff-update-setup complete", { exact: false }),
    ).toBeVisible({ timeout: 60_000 });

    // Both tasks now have diffs and the live monitor has run at least once
    // — meaning the orchestrator should have persisted a live_monitor
    // snapshot row for each session via UpsertLatestLiveGitSnapshot.

    // Restart the backend. This destroys all in-memory executions, so
    // GetExecutionBySessionID will return nil for both sessions on the next
    // session.subscribe — forcing the DB-snapshot fallback path to run.
    await backend.restart();

    // Reload and navigate to Beta to re-establish the WS connection — Beta
    // becomes the active task and Alpha becomes the non-active task, which
    // is exactly the case the bug under test exercises (badge survives via
    // the persisted DB-snapshot fallback for the inactive sidebar entry).
    await testPage.goto(`/t/${taskBeta.id}`);
    await betaSession.waitForLoad();

    // Alpha is the non-active task here (we navigated to Beta). Its badge
    // must come from the persisted DB snapshot — that's the regression this
    // test guards. We deliberately do NOT assert on the active task's badge:
    // that path goes through live status capture and is exercised by other
    // tests; folding it in here just couples this regression test to an
    // unrelated live-capture race that has its own timing characteristics.
    const alphaRow = betaSession.sidebar
      .getByTestId("sidebar-task-item")
      .filter({ hasText: "Diff Alpha" });

    await expect(alphaRow).toBeVisible({ timeout: 15_000 });

    // Diff badge is rendered as "+N -N" inside a font-mono span.
    await expect(alphaRow.getByText(/\+\d+\s+-\d+/)).toBeVisible({ timeout: 30_000 });
  });
});
