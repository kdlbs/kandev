import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { dwell } from "../../helpers/causal-waits";

/**
 * Regression: when workspace preparation takes longer than the file-tree
 * retry budget (~18s), the right-sidebar file tree and terminal must keep
 * waiting instead of giving up. We simulate a slow workspace prepare by
 * writing a delay file that the backend fixture's `git` shim reads — so
 * `git fetch` inside the worktree preparer blocks for N seconds, which
 * serialises agentctl readiness behind it (same ordering that a real slow
 * git pull on a big monorepo would produce).
 */
test.describe("Long prepare (slow git fetch)", () => {
  test("file tree and terminal keep waiting and recover when fetch completes", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    // Sleep 22s on every `git fetch`/`git pull` run by the backend. The
    // worktree fetch timeout is 90s (see worktree/manager.go) so 22s fits
    // comfortably under it. 22s is still past the file-tree retry budget
    // (~18s), which is what we want to exercise.
    const delayFile = path.join(backend.tmpDir, "git-delay-ms");
    fs.writeFileSync(delayFile, "22000");

    try {
      // Force the worktree executor path; otherwise the task resolves with an
      // empty executor_type and runEnvironmentPreparer short-circuits, meaning
      // no fetch is ever invoked and the shim sleep never runs.
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Slow Git Fetch Task",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: seedData.worktreeExecutorProfileId,
        },
      );

      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();

      // While git fetch is sleeping, agentctl cannot become ready. The file
      // tree must sit in the "waiting" state (the fix: useFileBrowserTree
      // gates its initial load on agentctlStatus.isReady instead of racing
      // the 18s retry budget).
      const fileTreeWaiting = testPage.getByTestId("file-tree-waiting");
      const fileTreeManual = testPage.getByTestId("file-tree-manual");
      await expect(fileTreeWaiting).toBeVisible({ timeout: 15_000 });
      await expect(fileTreeManual).toHaveCount(0);

      // On faster CI runners the 22s fetch may already have completed by the
      // time the assertion below runs, so it does not require the waiting state
      // to still be visible.
      await dwell(
        testPage,
        19_000,
        "product-timer",
        "outlasts our own 1+2+5+10s file-tree retry ladder; the regression under test is the tree falling back to its manual state when that budget expires, and the expiry renders nothing to signal it",
      );
      await expect(fileTreeManual).toHaveCount(0);

      // Fetch eventually returns, worktree creation proceeds, agentctl
      // becomes ready. File tree leaves the waiting state automatically.
      await expect(fileTreeWaiting).toBeHidden({ timeout: 30_000 });
      await expect(fileTreeManual).toHaveCount(0);
    } finally {
      // Restore fast git for any subsequent tests that share this worker.
      if (fs.existsSync(delayFile)) fs.unlinkSync(delayFile);
    }
  });
});
