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

    // Reload and navigate back to one task to re-establish the WS connection.
    // The sidebar bulk-subscribes to all primary sessions on connect.
    await testPage.goto(`/t/${taskAlpha.id}`);
    await alphaSession.waitForLoad();

    // Both Alpha (active) and Beta (non-active) should display +N/-N badges.
    // diff-update-setup leaves one modified file with 1 added line and 1
    // removed line, so we just assert the badge text pattern is present in
    // each task row rather than a specific number (mock-agent details may
    // change). The key assertion is that the badge appears at all for the
    // non-active task — which is the bug.
    const alphaRow = alphaSession.sidebar
      .getByTestId("sidebar-task-item")
      .filter({ hasText: "Diff Alpha" });
    const betaRow = alphaSession.sidebar
      .getByTestId("sidebar-task-item")
      .filter({ hasText: "Diff Beta" });

    await expect(alphaRow).toBeVisible({ timeout: 15_000 });
    await expect(betaRow).toBeVisible({ timeout: 15_000 });

    // Diff badge is rendered as "+N -N" inside a font-mono span.
    await expect(alphaRow.getByText(/\+\d+\s+-\d+/)).toBeVisible({ timeout: 30_000 });
    await expect(betaRow.getByText(/\+\d+\s+-\d+/)).toBeVisible({ timeout: 30_000 });
  });
});
