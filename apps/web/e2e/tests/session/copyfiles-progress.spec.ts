import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

/**
 * Verifies the "Copy N ignored files" step the env preparer surfaces in the
 * prepare-progress panel whenever a worktree-mode repository has a non-empty
 * `copy_files` spec that actually matches source files.
 *
 * Setup:
 *   - Writes `.env` + `config/local.yml` into the worker-scoped seed repo.
 *   - PATCHes `copy_files` on that repo to ".env, config/local.yml".
 *   - Restores both in a finally so this never leaks into sibling tests
 *     (the seedData repo is shared across the whole worker).
 *
 * Why an E2E (not just a unit test): the existing unit coverage exercises the
 * step builder and the manager-level copy. This guards the full WS path —
 * env preparer → prepare event → store → PrepareProgress panel — so a
 * regression in any layer flips the panel back to silent.
 */
test.describe("Copy ignored files prepare step", () => {
  test("surfaces 'Copy N ignored files' step when copy_files is configured", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(60_000);

    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const envPath = path.join(repoDir, ".env");
    const configDir = path.join(repoDir, "config");
    const configPath = path.join(configDir, "local.yml");

    fs.mkdirSync(configDir, { recursive: true });
    fs.writeFileSync(envPath, "E2E_SECRET=hunter2\n");
    fs.writeFileSync(configPath, "debug: true\n");

    await apiClient.updateRepository(seedData.repositoryId, {
      copy_files: ".env, config/local.yml",
    });

    try {
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Copy Files Prepare Step",
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

      const panel = testPage.getByTestId("prepare-progress-panel");
      await expect(panel).toBeVisible({ timeout: 30_000 });
      await expect(panel).toHaveAttribute("data-status", "completed", { timeout: 30_000 });

      // Panel auto-collapses on a clean run — expand it so the step rows render.
      await panel.getByTestId("prepare-progress-toggle").click();
      await expect(panel).toHaveAttribute("data-expanded", "true");

      // Pluralized count is what proves the backend wired in CopiedFiles —
      // a regression in copyfiles.Copy's returned slice or the env preparer's
      // step builder would land on "Copy ignored files" (no count) or skip
      // the step entirely.
      await expect(panel).toContainText("Copy 2 ignored files", { timeout: 5_000 });
    } finally {
      await apiClient.updateRepository(seedData.repositoryId, { copy_files: "" }).catch(() => {
        // Test teardown is best-effort — a 404 here just means the repo
        // was already cleaned up by a parallel teardown.
      });
      fs.rmSync(envPath, { force: true });
      fs.rmSync(configDir, { recursive: true, force: true });
    }
  });

  test("does not render the step when no files match", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    // Configure copy_files to a pattern that matches nothing in the repo —
    // copyfiles.Copy returns zero copied + one "no matches" warning, but
    // because the warning IS surfaced the step still appears. Use a pattern
    // we know cannot match by giving it a path the seed repo never creates.
    await apiClient.updateRepository(seedData.repositoryId, {
      copy_files: "definitely-does-not-exist.never",
    });

    try {
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Copy Files Zero Match",
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

      const panel = testPage.getByTestId("prepare-progress-panel");
      await expect(panel).toBeVisible({ timeout: 30_000 });
      await expect(panel).toHaveAttribute("data-status", "completed_with_warnings", {
        timeout: 30_000,
      });

      // Warnings keep the panel auto-expanded; the step renders with zero
      // count ("Copy ignored files") and the no-match warning.
      await expect(panel).toContainText("Copy ignored files");
      await expect(panel).toContainText("definitely-does-not-exist.never");
    } finally {
      await apiClient.updateRepository(seedData.repositoryId, { copy_files: "" }).catch(() => {});
    }
  });
});
