import path from "node:path";
import { test, expect } from "../../fixtures/test-base";

test.describe("GitHub PR task launcher", () => {
  test("opens the create task dialog in Remote mode when the matching repo is a stale task worktree", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const owner = "staleorg";
    const repo = "stalerepo";
    const prNumber = 1567;
    const prTitle = "Remote fallback PR";
    const prURL = `https://github.com/${owner}/${repo}/pull/${prNumber}`;

    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAddPRs([
      {
        number: prNumber,
        title: prTitle,
        state: "open",
        head_branch: "feature/remote-fallback",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: owner,
        repo_name: repo,
      },
    ]);

    await apiClient.createRepository(
      seedData.workspaceId,
      path.join(backend.tmpDir, ".kandev", "tasks", "pr-1541-fix-skip-cle_3bm", "stalerepo"),
      "main",
      {
        name: `${owner}/${repo}`,
        provider: "github",
        provider_owner: owner,
        provider_name: repo,
      },
    );

    await testPage.goto("/github");
    await expect(testPage.getByTestId("github-presets-scope-bar")).toBeVisible({
      timeout: 15_000,
    });
    await expect(testPage.getByTestId("pr-row")).toHaveCount(1, { timeout: 15_000 });

    const row = testPage.getByTestId("pr-row").filter({ hasText: prTitle });
    await row.getByTestId("pr-start-task-trigger").click();
    await testPage.locator('[data-testid="pr-start-task-preset"][data-preset-id="review"]').click();

    await expect(testPage.getByTestId("task-title-input")).toHaveValue(`Review: ${prTitle}`);
    await expect(testPage.getByTestId("source-mode-remote")).toHaveAttribute(
      "aria-checked",
      "true",
    );
    await expect(testPage.getByTestId("remote-repo-chip")).toHaveAttribute(
      "data-remote-url",
      prURL,
    );
    await expect(testPage.getByTestId("remote-repo-chip-trigger")).toContainText(
      `pull/${prNumber}`,
    );
    await expect(testPage.getByTestId("repo-chip")).toHaveCount(0);
  });
});
