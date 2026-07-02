import { execSync } from "node:child_process";
import fs from "node:fs";
import { test, expect } from "../../fixtures/test-base";
import { makeGitEnv } from "../../helpers/git-helper";

test.describe("Create task from GitHub PR list", () => {
  // Cold-start backend boots can race on the first test in a worker.
  test.describe.configure({ retries: 1 });

  test("starting a task from a PR opens Remote mode with the PR URL and head branch", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(60_000);

    // seedData is worker-scoped, so the workspace can accumulate repos across
    // test reruns. Use a unique owner/repo per invocation so matchRepo lands
    // on exactly the one we just created.
    const suffix = `${process.pid}-${Date.now()}`;
    const ownerSlug = `owner-${suffix}`;
    const repoSlug = `repo-${suffix}`;

    // Create a real local git repo so the branch chip can enumerate branches.
    // Both `main` and the PR head branch must exist locally — the chip lists
    // local branches, not the GitHub mock's branches.
    const repoDir = `${backend.tmpDir}/repos/pr-list-${suffix}`;
    fs.mkdirSync(repoDir, { recursive: true });
    const gitEnv = makeGitEnv(backend.tmpDir);
    execSync("git init -b main", { cwd: repoDir, env: gitEnv });
    execSync('git commit --allow-empty -m "init"', { cwd: repoDir, env: gitEnv });
    execSync("git checkout -b feature/from-pr-list", { cwd: repoDir, env: gitEnv });
    execSync('git commit --allow-empty -m "feature"', { cwd: repoDir, env: gitEnv });
    execSync("git checkout main", { cwd: repoDir, env: gitEnv });

    // Pre-seed a GitHub-backed repository that matches the mock PR's owner/repo.
    // PR launch must still use Remote mode so stale local task worktrees cannot
    // be selected by repository matching.
    await apiClient.createRepository(seedData.workspaceId, repoDir, "main", {
      name: `${ownerSlug}/${repoSlug}`,
      provider: "github",
      provider_owner: ownerSlug,
      provider_name: repoSlug,
    });

    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAddBranches(ownerSlug, repoSlug, [
      { name: "main" },
      { name: "feature/from-pr-list" },
    ]);
    await apiClient.mockGitHubAddPRs([
      {
        number: 77,
        title: "PR from GitHub page",
        state: "open",
        head_branch: "feature/from-pr-list",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: ownerSlug,
        repo_name: repoSlug,
      },
    ]);

    await testPage.goto("/github");

    // Wait for the seeded PR row to render. The "Review requested" default
    // preset matches because the mock user is the requested reviewer fallback
    // for the seeded PR. We scope by data-pr-number to avoid catching any
    // leftover PRs cached by previous tests.
    const prRow = testPage.locator('[data-testid="pr-row"][data-pr-number="77"]');
    await expect(prRow).toBeVisible({ timeout: 15_000 });

    // Open the task launcher menu for this PR and choose the "Review" preset.
    await prRow.getByTestId("pr-start-task-trigger").click();
    await testPage.locator('[data-testid="pr-start-task-preset"][data-preset-id="review"]').click();

    const prURL = `https://github.com/${ownerSlug}/${repoSlug}/pull/77`;

    // The create-task dialog should open in Remote mode with the PR URL and
    // head branch pre-filled.
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.getByTestId("source-mode-remote")).toHaveAttribute("aria-checked", "true");
    await expect(dialog.getByTestId("remote-repo-chip")).toHaveAttribute("data-remote-url", prURL);
    await expect(dialog.getByTestId("remote-repo-chip-trigger")).toContainText("pull/77");
    await expect(dialog.getByTestId("remote-branch-chip-trigger")).toContainText(
      "feature/from-pr-list",
    );
    await expect(dialog.getByTestId("repo-chip")).toHaveCount(0);

    // The dialog title should be derived from the preset + PR title.
    await expect(testPage.getByTestId("task-title-input")).toHaveValue(
      /Review: PR from GitHub page/,
    );
  });
});
