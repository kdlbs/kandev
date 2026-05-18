import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

/**
 * Regression test for a duplicate-execution bug where the per-repo setup
 * script (Repository.setup_script) ran twice during worktree prepare:
 *   1. Once by worktree.Manager.Create via its script handler (correct).
 *   2. Once by the env preparer when resolving the prepare script's
 *      {{repository.setup_script}} placeholder (wrong).
 *
 * For non-idempotent scripts this surfaces as the second run failing — the
 * env preparer treats it as a non-fatal step, so the prepare panel ends in
 * `completed_with_error` rather than `completed`. The fix blanks
 * RepoSetupScript on the request copy used to resolve the prepare script,
 * so the placeholder substitutes to empty and the second run is skipped.
 *
 * The unit-level coverage lives in
 * apps/backend/internal/agent/lifecycle/env_preparer_worktree_dup_test.go;
 * this test exercises the full HTTP + worktree manager + env preparer path.
 */
test.describe("Setup script no duplicate execution", () => {
  test("non-idempotent repository setup_script runs exactly once on worktree task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    // `mkdir <dir>` (without -p) fails with "File exists" on a second run.
    // Each worktree gets its own working directory, so the directory only
    // exists when the script ran twice in the same worktree — which is the
    // exact symptom of the duplicate-execution bug.
    const nonIdempotentScript = "mkdir build-once-marker";

    await apiClient.updateRepository(seedData.repositoryId, {
      setup_script: nonIdempotentScript,
    });

    try {
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Setup Script No Duplicate",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          // Default worktree profile uses the {{repository.setup_script}}
          // placeholder, which is the path the bug lived on.
          executor_profile_id: seedData.worktreeExecutorProfileId,
        },
      );

      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();

      const panel = testPage.getByTestId("prepare-progress-panel");
      await expect(panel).toBeVisible({ timeout: 30_000 });

      // Wait for prepare to settle into a terminal state.
      await expect(panel).toHaveAttribute("data-status", /completed|completed_with_error|failed/, {
        timeout: 45_000,
      });

      // Expand to see step content if auto-collapsed.
      if ((await panel.getAttribute("data-expanded")) === "false") {
        await panel.getByTestId("prepare-progress-toggle").click();
      }
      const text = await panel.innerText();

      // The setup script must be executed exactly once, by the worktree
      // manager (recorded as a `script_execution` message). The env
      // preparer must NOT also run it via its own "Run setup script"
      // prepare step — that's the duplicate-execution path the fix
      // closes. Two distinct assertions because the two runners surface
      // through different channels (prepare panel vs. session messages).
      expect(text).not.toContain("Run setup script");

      expect(task.session_id).toBeTruthy();
      const { messages } = await apiClient.listSessionMessages(task.session_id!);
      const scriptMsgs = messages.filter(
        (m) => (m as { type?: string }).type === "script_execution",
      );
      expect(scriptMsgs).toHaveLength(1);
    } finally {
      // Reset the seed repo so worker-scoped state stays clean for other
      // tests sharing the same repository.
      await apiClient.updateRepository(seedData.repositoryId, { setup_script: "" }).catch(() => {});
    }
  });
});
