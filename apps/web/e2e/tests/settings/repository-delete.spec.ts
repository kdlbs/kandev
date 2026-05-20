import path from "node:path";
import fs from "node:fs";
import { execSync } from "node:child_process";
import { test, expect } from "../../fixtures/test-base";
import { makeGitEnv } from "../../helpers/git-helper";

/**
 * Covers the pre-deletion guard on repositories.
 *
 * The card calls `GET /api/v1/repositories/:id/active-session-count` when the
 * user clicks Delete. When the count is zero the dialog offers the destructive
 * action; when the count is positive the destructive button is hidden and the
 * dialog explains the user must stop the active sessions first. This is the
 * UX parity with the workflow-delete flow.
 */
test.describe("Repository deletion", () => {
  async function makeRepo(opts: {
    apiClient: import("../../helpers/api-client").ApiClient;
    backendTmpDir: string;
    workspaceId: string;
    suffix: string;
  }): Promise<{ id: string; name: string }> {
    const repoName = `E2E Repo Delete ${opts.suffix}`;
    const repoDir = path.join(opts.backendTmpDir, "repos", `e2e-repo-delete-${opts.suffix}`);
    fs.mkdirSync(repoDir, { recursive: true });
    const env = makeGitEnv(opts.backendTmpDir);
    execSync("git init -b main", { cwd: repoDir, env });
    execSync('git commit --allow-empty -m "init"', { cwd: repoDir, env });
    const repo = await opts.apiClient.createRepository(opts.workspaceId, repoDir, "main", {
      name: repoName,
    });
    return { id: repo.id, name: repoName };
  }

  test("removes the card after confirm when no active sessions reference the repository", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(60_000);

    // testPage runs e2eReset before every test/retry, so a static suffix is
    // safe and keeps test output deterministic.
    const suffix = "ok";
    const repo = await makeRepo({
      apiClient,
      backendTmpDir: backend.tmpDir,
      workspaceId: seedData.workspaceId,
      suffix,
    });

    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/repositories`);

    const card = testPage.locator('[data-slot="card"]', { hasText: repo.name });
    await expect(card).toBeVisible({ timeout: 15_000 });

    await card.getByRole("button", { name: "Delete", exact: true }).click();

    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible({ timeout: 10_000 });
    await expect(dialog.getByText(/This action cannot be undone/i)).toBeVisible();
    await expect(dialog.getByRole("button", { name: "Delete Repository" })).toBeVisible();

    await dialog.getByRole("button", { name: "Delete Repository" }).click();

    await expect(card).toBeHidden({ timeout: 10_000 });
    await expect(dialog).toBeHidden({ timeout: 10_000 });
  });

  test("blocks deletion and hides destructive button when an active session uses the repository", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const suffix = "blocked";
    const repo = await makeRepo({
      apiClient,
      backendTmpDir: backend.tmpDir,
      workspaceId: seedData.workspaceId,
      suffix,
    });

    // Long delay keeps the mock-agent in RUNNING for the whole test so the
    // CountActiveTaskSessionsByRepository query is guaranteed to return 1.
    // e2eReset on the next test tears the task down via DeleteTask, which
    // shuts the agentctl instance via the cleanup goroutine.
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      `Repo Delete Blocker ${suffix}`,
      seedData.agentProfileId,
      {
        description: 'e2e:delay(60000)\ne2e:message("done")',
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [repo.id],
      },
    );

    // Wait until the count endpoint actually reports the session as active —
    // there is a small window between task create, session insert, and the
    // join-against-task_repositories query observing the row. Without this
    // poll the click can race the SQL and the dialog opens in the
    // no-active-sessions branch.
    await expect
      .poll(
        async () => {
          const res = await apiClient.rawRequest(
            "GET",
            `/api/v1/repositories/${repo.id}/active-session-count`,
          );
          if (!res.ok) return -1;
          const body = (await res.json()) as { active_session_count: number };
          return body.active_session_count;
        },
        { timeout: 15_000, intervals: [200, 500, 1_000] },
      )
      .toBeGreaterThan(0);

    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/repositories`);

    const card = testPage.locator('[data-slot="card"]', { hasText: repo.name });
    await expect(card).toBeVisible({ timeout: 15_000 });

    await card.getByRole("button", { name: "Delete", exact: true }).click();

    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible({ timeout: 10_000 });
    await expect(dialog.getByText(/active agent session/i)).toBeVisible();

    // Destructive button is gated behind hasActiveSessions === false, so the
    // count > 0 path must not render it.
    await expect(dialog.getByRole("button", { name: "Delete Repository" })).toHaveCount(0);

    // Radix DialogContent renders its own X close icon also labelled "Close";
    // scope to the footer slot so the locator resolves strictly to the visible
    // footer button instead of silently disambiguating with .first().
    const closeButton = dialog
      .locator('[data-slot="dialog-footer"]')
      .getByRole("button", { name: "Close", exact: true });
    await expect(closeButton).toBeVisible();
    await closeButton.click();
    await expect(dialog).toBeHidden({ timeout: 5_000 });

    // The card stays — the repository was not deleted.
    await expect(card).toBeVisible();
  });
});
