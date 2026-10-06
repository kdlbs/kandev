import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { dwell } from "../../helpers/causal-waits";

/**
 * Regression: when workspace preparation takes longer than the file-tree
 * retry budget (~18s), the right-sidebar file tree and terminal must keep
 * waiting instead of giving up. The backend fixture's `git` shim gates
 * `fetch` until the test releases it. This serializes agentctl readiness
 * behind workspace preparation, matching a slow Git operation in a large
 * repository.
 */
test.describe("Long prepare (slow git fetch)", () => {
  test.afterEach(async ({ backend }, testInfo) => {
    if (testInfo.status === testInfo.expectedStatus) return;
    await testInfo.attach("long-prepare-backend.log", {
      path: backend.logPath,
      contentType: "text/plain",
    });
  });

  test("file tree and terminal keep waiting and recover when fetch completes", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    // Hold the actual fetch so the file-tree retry budget is measured from the
    // preparation boundary, not from task creation or page navigation.
    const gateFile = path.join(backend.tmpDir, "git-delay-ms");
    const startedFile = path.join(backend.tmpDir, "git-fetch-started");
    const releaseFile = path.join(backend.tmpDir, "git-fetch-release");
    const repositoryResponse = await apiClient.rawRequest(
      "GET",
      `/api/v1/repositories/${seedData.repositoryId}`,
    );
    expect(repositoryResponse.ok).toBeTruthy();
    const repository = (await repositoryResponse.json()) as { pull_before_worktree?: boolean };

    try {
      // The worker repository is shared across specs. Fetch must be enabled
      // explicitly so earlier repository edits cannot bypass this gate.
      fs.writeFileSync(gateFile, JSON.stringify({ startedFile, releaseFile, subcommand: "fetch" }));
      await apiClient.updateRepository(seedData.repositoryId, { pull_before_worktree: true });
      // Force the worktree executor path; otherwise the task resolves with an
      // empty executor_type and runEnvironmentPreparer short-circuits, meaning
      // no fetch is ever invoked and the shim gate never starts.
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
      // Persisted layouts can leave Changes active; mount Files before observing its loader.
      await session.clickTab("Files");

      await expect
        .poll(() => fs.existsSync(startedFile), {
          timeout: 30_000,
          message: "workspace preparation should reach the gated Git fetch",
        })
        .toBe(true);

      // While the fetch is held, agentctl cannot become ready and the file
      // tree must stay in its waiting state beyond its retry budget.
      const fileTreeWaiting = testPage.getByTestId("file-tree-waiting");
      const fileTreeManual = testPage.getByTestId("file-tree-manual");
      await expect(fileTreeWaiting).toBeVisible();
      await expect(fileTreeManual).toHaveCount(0);

      await dwell(
        testPage,
        19_000,
        "product-timer",
        "outlasts our own 1+2+5+10s file-tree retry ladder; the regression under test is the tree falling back to its manual state when that budget expires, and the expiry renders nothing to signal it",
      );
      await expect(fileTreeWaiting).toBeVisible();
      await expect(fileTreeManual).toHaveCount(0);

      // Fetch eventually returns, worktree creation proceeds, agentctl
      // becomes ready. File tree leaves the waiting state automatically.
      fs.writeFileSync(releaseFile, "release");
      await expect(fileTreeWaiting).toBeHidden({ timeout: 30_000 });
      await expect(fileTreeManual).toHaveCount(0);
      await session.clickTab("Files");
      await expect(testPage.getByTestId("file-tree-node").first()).toBeVisible({ timeout: 30_000 });
    } finally {
      // Always release Git before removing the gate, including failed tests.
      fs.writeFileSync(releaseFile, "release");
      try {
        await apiClient.updateRepository(seedData.repositoryId, {
          pull_before_worktree: repository.pull_before_worktree === true,
        });
      } finally {
        if (fs.existsSync(gateFile)) fs.unlinkSync(gateFile);
        if (fs.existsSync(startedFile)) fs.unlinkSync(startedFile);
        if (fs.existsSync(releaseFile)) fs.unlinkSync(releaseFile);
      }
    }
  });
});
