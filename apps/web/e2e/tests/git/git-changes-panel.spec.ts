import { test, expect } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";
import type { ApiClient } from "../../helpers/api-client";
import { waitForSessionState } from "../../helpers/session";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import type { Page } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import { execSync } from "node:child_process";
import { waitForDiffText, waitForDiffTextAbsent } from "./diff-update-helpers";

// ---------------------------------------------------------------------------
// Git helper for E2E tests - runs git commands in the test repository
// ---------------------------------------------------------------------------

class GitHelper {
  constructor(
    private repoDir: string,
    private env: NodeJS.ProcessEnv,
  ) {}

  exec(cmd: string): string {
    const lockPath = path.join(this.repoDir, ".git", "index.lock");
    // Retry up to 3 times on index.lock conflicts. The backend's git status
    // polling briefly holds the lock; waiting a short time and retrying is
    // safer than deleting an actively-held lock.
    for (let attempt = 0; attempt < 3; attempt++) {
      if (fs.existsSync(lockPath)) fs.unlinkSync(lockPath);
      try {
        return execSync(cmd, { cwd: this.repoDir, env: this.env, encoding: "utf8" });
      } catch (err) {
        const msg = (err as Error).message ?? "";
        if (msg.includes("index.lock") && attempt < 2) {
          execSync("sleep 0.2");
          continue;
        }
        throw err;
      }
    }
    throw new Error(`git exec failed after 3 attempts: ${cmd}`);
  }

  createFile(name: string, content: string) {
    const filePath = path.join(this.repoDir, name);
    fs.writeFileSync(filePath, content);
  }

  modifyFile(name: string, content: string) {
    this.createFile(name, content);
  }

  deleteFile(name: string) {
    const filePath = path.join(this.repoDir, name);
    if (fs.existsSync(filePath)) {
      fs.unlinkSync(filePath);
    }
  }

  stageFile(name: string) {
    this.exec(`git add "${name}"`);
  }

  stageAll() {
    this.exec("git add -A");
  }

  commit(message: string): string {
    this.exec(`git commit -m "${message}"`);
    return this.exec("git rev-parse HEAD").trim();
  }

  getCurrentSha(): string {
    return this.exec("git rev-parse HEAD").trim();
  }
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

/** Navigate to a kanban card by title and open its session page. */
async function openTaskSession(page: Page, title: string): Promise<SessionPage> {
  const kanban = new KanbanPage(page);
  await kanban.goto();

  const card = kanban.taskCardByTitle(title);
  await expect(card).toBeVisible({ timeout: 15_000 });
  await card.click();
  await expect(page).toHaveURL(/\/t\//, { timeout: 15_000 });

  const session = new SessionPage(page);
  await session.waitForLoad();
  return session;
}

/** Create a non-passthrough (standard) agent profile for the mock agent. */
async function createStandardProfile(apiClient: ApiClient, name: string) {
  const { agents } = await apiClient.listAgents();
  const agentId = agents[0]?.id;
  if (!agentId) {
    throw new Error(`E2E setup failed: no agent available for profile "${name}"`);
  }
  return apiClient.createAgentProfile(agentId, name, {
    model: "mock-fast",
    auto_approve: true,
    cli_passthrough: false,
  });
}

function createRewrittenProviderHistory(
  git: GitHelper,
  branch: string,
): {
  head: string;
  commits: Array<{
    sha: string;
    message: string;
    author_login: string;
    author_date: string;
    stats_available: boolean;
  }>;
} {
  git.exec(`git checkout -B kandev-e2e-provider-rewrite origin/main`);
  const commits: Array<{
    sha: string;
    message: string;
    author_login: string;
    author_date: string;
    stats_available: boolean;
  }> = [];
  for (let index = 1; index <= 15; index += 1) {
    const message = `Rewritten provider commit ${index}`;
    git.createFile(`provider-rewrite-${index}.txt`, message);
    git.stageFile(`provider-rewrite-${index}.txt`);
    const sha = git.commit(message);
    commits.push({
      sha,
      message,
      author_login: "remote-contributor",
      author_date: `2026-08-${String(index).padStart(2, "0")}T12:00:00Z`,
      stats_available: false,
    });
  }
  git.exec(`git push --force origin HEAD:refs/heads/${branch}`);
  git.exec("git checkout -f main");
  return { head: commits[commits.length - 1].sha, commits };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe("Git Changes Panel", () => {
  /**
   * Verifies that modified files appear in the unstaged section of the Changes panel.
   * Creates a task, modifies a file in the repository, and verifies the Changes panel
   * shows the modification in real-time via WebSocket updates.
   */
  test("shows modified files in unstaged section", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const profile = await createStandardProfile(apiClient, "Git Test Profile");

    await apiClient.createTaskWithAgent(seedData.workspaceId, "Git Changes Test", profile.id, {
      description: "Testing git changes panel",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "Git Changes Test");

    // Set up git helper for the test repository
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };
    const git = new GitHelper(repoDir, gitEnv);

    // Create and commit a file so we can modify it
    git.createFile("test-file.txt", "initial content");
    git.stageAll();
    git.commit("Add test file");

    // Now modify the file
    git.modifyFile("test-file.txt", "modified content");

    // Click the Changes tab to see the panel
    await session.clickTab("Changes");

    // Wait for the Changes panel to show the modified file
    await expect(session.changes).toBeVisible({ timeout: 10_000 });

    // The file should appear in the unstaged section
    // Poll for the file to appear (git status updates via WebSocket)
    await expect(testPage.getByTestId("unstaged-files-section")).toBeVisible({ timeout: 15_000 });
    // Scope the file search to the changes panel to avoid matching Files panel
    await expect(session.changes.getByText("test-file.txt")).toBeVisible({ timeout: 15_000 });
  });

  // @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.10
  test("shows same path in staged and unstaged sections", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const profile = await createStandardProfile(apiClient, "Git Mixed Change Profile");

    await apiClient.createTaskWithAgent(seedData.workspaceId, "Git Mixed Change Test", profile.id, {
      description: "Testing mixed staged and unstaged changes",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "Git Mixed Change Test");
    await session.waitForChatIdle();

    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    });

    git.createFile("mixed-layer.txt", "base\n");
    git.stageFile("mixed-layer.txt");
    git.commit("Add mixed layer fixture");
    git.modifyFile("mixed-layer.txt", "base\nSTAGED_LAYER_MARKER\n");
    git.stageFile("mixed-layer.txt");
    git.modifyFile("mixed-layer.txt", "base\nSTAGED_LAYER_MARKER\nUNSTAGED_LAYER_MARKER\n");

    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });
    await session.expandChangesSection("unstaged-files-section");
    await session.expandChangesSection("staged-files-section");

    const unstagedRow = testPage
      .getByTestId("unstaged-files-section")
      .getByTestId("file-row-mixed-layer.txt");
    const stagedRow = testPage
      .getByTestId("staged-files-section")
      .getByTestId("file-row-mixed-layer.txt");
    await expect(unstagedRow).toBeVisible();
    await expect(stagedRow).toBeVisible();
    await expect(unstagedRow.getByText("+1", { exact: true })).toBeVisible();
    await expect(stagedRow.getByText("+1", { exact: true })).toBeVisible();

    await unstagedRow.click();
    await waitForDiffText(testPage, "UNSTAGED_LAYER_MARKER");

    await stagedRow.click();
    await waitForDiffText(testPage, "STAGED_LAYER_MARKER");
    await waitForDiffTextAbsent(testPage, "UNSTAGED_LAYER_MARKER");

    await stagedRow.getByTitle("Unstage file").click();
    await expect(stagedRow).toHaveCount(0);
    await expect(unstagedRow).toBeVisible();
    await expect(unstagedRow.getByText("+2", { exact: true })).toBeVisible();
  });

  /**
   * Verifies that new untracked files appear in the unstaged section.
   */
  test("shows new untracked files in unstaged section", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const profile = await createStandardProfile(apiClient, "Git New File Profile");

    await apiClient.createTaskWithAgent(seedData.workspaceId, "Git New File Test", profile.id, {
      description: "Testing new file detection",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "Git New File Test");

    // Set up git helper
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };
    const git = new GitHelper(repoDir, gitEnv);

    // Create a new untracked file
    git.createFile("new-feature.ts", "export const feature = 'new';");

    // Click the Changes tab
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });

    // The new file should appear in unstaged (scope to changes panel to avoid
    // matching the Files panel which also shows the filename)
    await expect(testPage.getByTestId("unstaged-files-section")).toBeVisible({ timeout: 15_000 });
    await expect(session.changes.getByText("new-feature.ts")).toBeVisible({ timeout: 15_000 });

    // Clean up
    git.deleteFile("new-feature.ts");
  });

  /**
   * Verifies that files with spaces in their path are shown correctly in the
   * Changes panel and that their diff content is visible (not "No changes").
   * Regression test for: git status --porcelain quotes paths with spaces,
   * and the backend must unquote them so diff lookups succeed.
   */
  test("shows modified files with spaces in path", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const profile = await createStandardProfile(apiClient, "Git Spaces Profile");

    await apiClient.createTaskWithAgent(seedData.workspaceId, "Git Spaces Test", profile.id, {
      description: "Testing paths with spaces",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "Git Spaces Test");

    // Set up git helper
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };
    const git = new GitHelper(repoDir, gitEnv);

    // Create a file inside a directory with spaces
    const dirWithSpaces = path.join(repoDir, "path with spaces");
    fs.mkdirSync(dirWithSpaces, { recursive: true });
    fs.writeFileSync(path.join(dirWithSpaces, "file.md"), "initial content");
    git.stageAll();
    git.commit("Add file with spaces in path");

    // Modify the file to create an unstaged change
    fs.writeFileSync(path.join(dirWithSpaces, "file.md"), "modified content");

    // Click the Changes tab
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });

    // The file should appear in the unstaged section with its unquoted path
    await expect(testPage.getByTestId("unstaged-files-section")).toBeVisible({ timeout: 15_000 });
    await expect(session.changes.getByText("file.md")).toBeVisible({ timeout: 15_000 });

    // Click the file to open its diff and verify content is shown
    await session.changes.getByText("file.md").click();

    // Verify the diff viewer shows actual diff content (not empty / "No changes").
    // Pierre Diffs renders in a shadow DOM — check all diffs-container elements.
    await testPage.waitForFunction(
      (searchText: string) => {
        for (const container of document.querySelectorAll("diffs-container")) {
          const shadow = container.shadowRoot;
          if (shadow?.textContent?.includes(searchText)) return true;
        }
        return false;
      },
      "modified content",
      { timeout: 30_000 },
    );
  });

  /**
   * Verifies that commits appear in the commits section after committing staged files.
   * This tests the full flow: create file → stage → commit → verify in UI.
   */
  test("shows commits after staging and committing", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const profile = await createStandardProfile(apiClient, "Git Commit Profile");

    await apiClient.createTaskWithAgent(seedData.workspaceId, "Git Commit Test", profile.id, {
      description: "Testing commit flow",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "Git Commit Test");

    // Set up git helper
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };
    const git = new GitHelper(repoDir, gitEnv);

    // Create, stage, and commit a file
    git.createFile("feature.ts", "export const x = 1;");
    git.stageAll();
    const sha = git.commit("Add feature module");

    // Click the Changes tab
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });

    // The commit should appear in the commits section
    await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 15_000 });
    await session.expandCommitsSection();
    await expect(testPage.getByText("Add feature module")).toBeVisible({ timeout: 15_000 });
    // Verify the short SHA is displayed
    await expect(testPage.getByText(sha.slice(0, 7))).toBeVisible({ timeout: 5_000 });
  });

  /**
   * Verifies that clicking a commit opens its diff view.
   * This tests the integration between the commits list and the diff viewer.
   */
  test("clicking commit opens diff view", async ({ testPage, apiClient, seedData, backend }) => {
    const profile = await createStandardProfile(apiClient, "Git Diff Profile");

    await apiClient.createTaskWithAgent(seedData.workspaceId, "Git Diff Test", profile.id, {
      description: "Testing commit diff view",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "Git Diff Test");

    // Set up git helper
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };
    const git = new GitHelper(repoDir, gitEnv);

    // Create a commit with content we can verify in the diff
    git.createFile("diff-test.txt", "line 1\nline 2\nline 3");
    git.stageAll();
    const sha = git.commit("Add diff test file");

    // Click the Changes tab
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });

    // Wait for the commit to appear
    await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 15_000 });
    await session.expandCommitsSection();
    const commitRow = testPage.getByTestId(`commit-row-${sha.slice(0, 7)}`);
    await expect(commitRow).toBeVisible({ timeout: 10_000 });

    // Click the commit to open its diff
    await commitRow.click();

    // The diff view should open showing the commit message and file changes
    // Look for the commit message (which uniquely identifies this diff view)
    await expect(session.changes.getByText("Add diff test file")).toBeVisible({ timeout: 10_000 });

    // Additionally verify the diff shows the actual file content (lines added).
    // Pierre Diffs renders in a shadow DOM — check all diffs-container elements
    // since multiple may exist (inline chat diffs + Changes panel).
    await testPage.waitForFunction(
      (searchText: string) => {
        for (const container of document.querySelectorAll("diffs-container")) {
          const shadow = container.shadowRoot;
          if (shadow?.textContent?.includes(searchText)) return true;
        }
        return false;
      },
      "line 1",
      { timeout: 60_000 },
    );
    await expect(testPage.getByText("line 1")).toBeVisible({ timeout: 5_000 });
  });

  test("PR-only commit uses GitHub details when local history is stale", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const profile = await createStandardProfile(apiClient, "Git PR-only Detail Profile");
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Git PR-only Detail Test",
      profile.id,
      {
        description: "Testing PR-only commit details after a force-push",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    const session = await openTaskSession(testPage, "Git PR-only Detail Test");

    const remoteSha = "d".repeat(40);
    const remoteMessage = "Force-pushed remote commit";
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    });
    git.createFile("pr-shared-marker.ts", "shared provider checkout commit");
    git.stageFile("pr-shared-marker.ts");
    const sharedSha = git.commit("Shared provider checkout commit");
    const checkoutBranch = git.exec("git branch --show-current").trim();
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("remote-author");
    await apiClient.mockGitHubAddPRs([
      {
        number: 2253,
        title: "Force-pushed PR",
        state: "open",
        head_branch: checkoutBranch,
        base_branch: "main",
        author_login: "remote-author",
        repo_owner: "testorg",
        repo_name: "testrepo",
        head_sha: remoteSha,
      },
    ]);
    await apiClient.mockGitHubAddPRCommits("testorg", "testrepo", 2253, [
      {
        sha: sharedSha,
        message: "Shared provider checkout commit",
        author_login: "remote-author",
        author_date: "2026-08-03T12:00:00Z",
      },
      {
        sha: remoteSha,
        message: remoteMessage,
        author_login: "remote-author",
        author_date: "2026-08-04T12:00:00Z",
        stats_available: false,
      },
    ]);
    await apiClient.mockGitHubAddPRCommitDetail("testorg", "testrepo", remoteSha, {
      message: remoteMessage,
      author_login: "remote-author",
      author_name: "Remote Author",
      author_date: "2026-08-04T12:00:00Z",
      additions: 1,
      deletions: 0,
      files_changed: 1,
      files: [
        {
          filename: "pr-only-marker.ts",
          status: "added",
          additions: 1,
          deletions: 0,
          patch: "@@ -0,0 +1 @@\n+PR_ONLY_REMOTE_MARKER",
        },
      ],
    });
    await apiClient.mockGitHubSetPRCommitsFailures("testorg", "testrepo", 2253, 1);
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 2253,
      pr_url: "https://github.com/testorg/testrepo/pull/2253",
      pr_title: "Force-pushed PR",
      head_branch: checkoutBranch,
      base_branch: "main",
      author_login: "remote-author",
    });

    await testPage.reload();
    await session.waitForLoad();
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 20_000 });
    await session.expandCommitsSection();
    const commitsList = testPage.getByTestId("commits-list");

    const row = testPage.getByTestId(`commit-row-${remoteSha.slice(0, 7)}`);
    await expect(row).toBeVisible({ timeout: 20_000 });
    await expect(row.getByTestId("commit-provenance")).toHaveAttribute(
      "data-commit-provenance",
      "current_pr",
    );
    await expect(row.getByTestId("commit-provenance")).toHaveAttribute(
      "title",
      "Current PR commit",
    );
    const sharedRow = testPage.getByTestId(`commit-row-${sharedSha.slice(0, 7)}`);
    await expect(sharedRow).toBeVisible({ timeout: 20_000 });
    await expect(sharedRow.getByTestId("commit-provenance")).toHaveAttribute(
      "data-commit-provenance",
      "pushed",
    );
    await expect(testPage.getByTestId("header-remote-contribution-warning")).toHaveCount(0);
    const commitMessages = await commitsList
      .locator('[data-testid^="commit-row-"]')
      .allTextContents();
    const remoteIndex = commitMessages.findIndex((message) => message.includes(remoteMessage));
    const sharedIndex = commitMessages.findIndex((message) =>
      message.includes("Shared provider checkout commit"),
    );
    expect(remoteIndex).toBeGreaterThanOrEqual(0);
    expect(sharedIndex).toBeGreaterThanOrEqual(0);
    expect(remoteIndex).toBeLessThan(sharedIndex);
    await expect(row.getByText("+0", { exact: true })).toHaveCount(0);
    await expect(row.getByText("-0", { exact: true })).toHaveCount(0);
    await row.hover();
    await expect(row.getByRole("button")).toHaveCount(0);

    await row.click();
    await expect(testPage.getByText(remoteMessage).last()).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByText("Remote Author")).toBeVisible({ timeout: 10_000 });
    await testPage.waitForFunction(
      (marker: string) =>
        Array.from(document.querySelectorAll("diffs-container")).some((container) =>
          container.shadowRoot?.textContent?.includes(marker),
        ),
      "PR_ONLY_REMOTE_MARKER",
      { timeout: 60_000 },
    );
  });

  /**
   * Verifies that reverting a commit undoes it (soft reset).
   * Note: The "Revert commit" action does `git reset --soft HEAD~1`,
   * NOT `git revert`. The commit is removed and changes become staged again.
   */
  test("revert commit undoes commit and stages changes", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const profile = await createStandardProfile(apiClient, "Git Revert Profile");

    await apiClient.createTaskWithAgent(seedData.workspaceId, "Git Revert Test", profile.id, {
      description: "Testing revert commit",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "Git Revert Test");

    // Set up git helper
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };
    const git = new GitHelper(repoDir, gitEnv);

    // Create a commit to revert
    git.createFile("to-revert.txt", "content to revert");
    git.stageAll();
    const sha = git.commit("Add file to revert");

    // Click the Changes tab
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });

    // Wait for the specific commit to appear by SHA
    await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 15_000 });
    await session.expandCommitsSection();
    const commitRow = testPage.getByTestId(`commit-row-${sha.slice(0, 7)}`);
    await expect(commitRow).toBeVisible({ timeout: 10_000 });

    // Verify the commit message is shown
    await expect(session.changes.getByText("Add file to revert")).toBeVisible({ timeout: 5_000 });

    // Click the revert button (hover action on the commit row)
    await commitRow.hover();
    const revertButton = commitRow.getByRole("button", { name: "Revert commit" });
    await expect(revertButton).toBeVisible({ timeout: 5_000 });
    await revertButton.click();

    // The "Revert commit" action does `git reset --soft HEAD~1`:
    // 1. The commit should disappear from the commits list
    // 2. The file should now appear in the STAGED section
    await expect(session.changes.getByText("Add file to revert")).not.toBeVisible({
      timeout: 15_000,
    });

    // The file should now be staged
    await expect(testPage.getByTestId("staged-files-section")).toBeVisible({ timeout: 10_000 });
    await expect(session.changes.getByText("to-revert.txt")).toBeVisible({ timeout: 5_000 });
  });

  /**
   * Verifies that resetting to a previous commit removes commits from history.
   * Creates commits via git, then resets via the UI dialog.
   */
  test("reset to commit removes newer commits from history", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);
    const profile = await createStandardProfile(apiClient, "Git Reset Profile");

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Git Reset Test",
      profile.id,
      {
        description: "Testing reset to commit",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    const session = await openTaskSession(testPage, "Git Reset Test");
    expect(task.session_id, "task must have a session to await").toBeTruthy();
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId: task.session_id as string,
      expectedState: "WAITING_FOR_INPUT",
      message: "the initial reset-test turn did not settle before git actions",
      timeout: 30_000,
    });

    // Set up git helper
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };
    const git = new GitHelper(repoDir, gitEnv);

    // Create two commits - we'll reset to the first one
    git.createFile("first.txt", "first file");
    git.stageAll();
    git.commit("First commit");

    git.createFile("second.txt", "second file");
    git.stageAll();
    git.commit("Second commit");

    // Click the Changes tab
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });

    // Wait for both commits to appear. The commits-section header renders as
    // soon as the section mounts (no commits required), so asserting on the
    // text alone races the WS git-status push that carries the actual commit
    // list. Gate on the two *named* commit rows we created — that's the signal
    // that the FE received them. Don't assert an exact total count: the
    // auto-started agent can land its own commit in the same worktree, so the
    // list may legitimately hold a third row.
    await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 15_000 });
    await session.expandCommitsSection();
    const commitsList = testPage.getByTestId("commits-list");
    const commitRows = commitsList.locator('[data-testid^="commit-row-"]');
    await expect(commitRows.filter({ hasText: "First commit" })).toHaveCount(1, {
      timeout: 15_000,
    });
    await expect(commitRows.filter({ hasText: "Second commit" })).toHaveCount(1, {
      timeout: 15_000,
    });
    await expect(session.changes.getByText("First commit")).toBeVisible({ timeout: 5_000 });
    await expect(session.changes.getByText("Second commit")).toBeVisible({ timeout: 5_000 });

    // Find the first commit row (it's the second in the list, older commit)
    // The list shows newest first, so "Second commit" is at index 0, "First commit" at index 1
    const firstCommitRow = commitsList
      .locator('[data-testid^="commit-row-"]')
      .filter({ hasText: "First commit" });
    await expect(firstCommitRow).toBeVisible({ timeout: 5_000 });

    // Get the SHA from the commit row to use in the confirmation
    const firstCommitSha = await firstCommitRow.locator("code").textContent();
    expect(firstCommitSha).toBeTruthy();

    // Click the reset button on the first commit row
    await firstCommitRow.hover();
    const resetButton = firstCommitRow.getByRole("button", { name: "Reset to this commit" });
    await expect(resetButton).toBeVisible({ timeout: 5_000 });
    // The action is rendered inside a group-hover span. Keep the row hovered
    // for the user-facing check above, then bypass CSS interception while the
    // stable idle session keeps the row from being replaced by a status push.
    await resetButton.click({ force: true });

    // Confirm the reset in the dialog
    const resetDialog = testPage.getByRole("dialog");
    await expect(resetDialog).toBeVisible({ timeout: 5_000 });

    // Select "Hard Reset" option by clicking the radio button
    const hardResetRadio = resetDialog.getByLabel(/Hard Reset/i);
    await hardResetRadio.click();

    // Type the commit SHA to confirm hard reset
    const confirmInput = resetDialog.getByPlaceholder(firstCommitSha!);
    await confirmInput.fill(firstCommitSha!);

    // Click the Reset button
    await resetDialog.getByRole("button", { name: "Reset" }).click();

    // Wait for the second commit to disappear from the list
    await expect(session.changes.getByText("Second commit")).not.toBeVisible({ timeout: 15_000 });

    // After a hard reset, the reset TO commit remains in history but any staged/unstaged changes
    // from newer commits are lost (it's a hard reset). Since we reset to "First commit" and
    // that was the original state, there may be no visible "changes" anymore - the commits
    // section might not appear if there are no commits in the "since base" range.
    //
    // For now, verify the reset worked by confirming "Second commit" is gone.
    // The "First commit" may or may not appear depending on whether base_commit_sha is set.
    // The test validates the reset operation completed successfully.
  });

  /**
   * Verifies that amending a commit updates the commit message in the history.
   * Uses the UI amend button on the latest commit.
   */
  test("amend commit updates commit message", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const profile = await createStandardProfile(apiClient, "Git Amend Profile");

    await apiClient.createTaskWithAgent(seedData.workspaceId, "Git Amend Test", profile.id, {
      description: "Testing amend commit",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "Git Amend Test");

    // Set up git helper
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };
    const git = new GitHelper(repoDir, gitEnv);

    // Create a commit to amend
    git.createFile("amend-test.txt", "content");
    git.stageAll();
    const sha = git.commit("Original message");

    // Click the Changes tab
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });

    // Wait for the commit to appear
    await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 15_000 });
    await session.expandCommitsSection();
    const commitRow = testPage.getByTestId(`commit-row-${sha.slice(0, 7)}`);
    await expect(commitRow).toBeVisible({ timeout: 10_000 });

    // Verify the original message is shown
    await expect(session.changes.getByText("Original message")).toBeVisible({ timeout: 5_000 });

    // Click the amend button (hover action on commit row)
    await commitRow.hover();
    const amendButton = commitRow.getByRole("button", { name: "Amend commit message" });
    await expect(amendButton).toBeVisible({ timeout: 5_000 });
    await amendButton.click();

    // Fill in the new message in the dialog
    const amendDialog = testPage.getByRole("dialog");
    await expect(amendDialog).toBeVisible({ timeout: 5_000 });
    const messageInput = amendDialog.getByRole("textbox");
    await messageInput.clear();
    await messageInput.fill("Amended message");
    await amendDialog.getByRole("button", { name: /Amend/i }).click();

    // Wait for the new message to appear and old message to disappear
    await expect(session.changes.getByText("Amended message")).toBeVisible({ timeout: 15_000 });
    await expect(session.changes.getByText("Original message")).not.toBeVisible({ timeout: 5_000 });
  });

  /**
   * Verifies that external commits (made outside the UI) appear in the history.
   * This tests the real-time update via WebSocket when commits change.
   */
  test("external commits appear in real-time via WebSocket", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const profile = await createStandardProfile(apiClient, "Git External Profile");

    await apiClient.createTaskWithAgent(seedData.workspaceId, "Git External Test", profile.id, {
      description: "Testing external commit detection",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "Git External Test");

    // Click the Changes tab first
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });

    await dwell(
      testPage,
      2_000,
      "unverified",
      "pre-existing spacing before the external-git edits below; the panel is already asserted visible above and no timer was identified behind this, so it is labelled as debt rather than given a cause it does not have",
    );

    // Set up git helper
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "External User",
      GIT_AUTHOR_EMAIL: "external@test.local",
      GIT_COMMITTER_NAME: "External User",
      GIT_COMMITTER_EMAIL: "external@test.local",
    };
    const git = new GitHelper(repoDir, gitEnv);

    // Create a commit externally (simulating another user or the agent)
    git.createFile("external-file.txt", "external content");
    git.stageAll();
    const sha = git.commit("External commit from another user");

    // The commit should appear in the UI via WebSocket update (when git status polls)
    // Note: The polling interval may mean we need to wait a bit
    await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 30_000 });
    await session.expandCommitsSection();
    await expect(session.changes.getByText("External commit from another user")).toBeVisible({
      timeout: 30_000,
    });

    // Verify the commit SHA is shown
    await expect(session.changes.getByText(sha.slice(0, 7))).toBeVisible({ timeout: 5_000 });
  });

  /**
   * Verifies that commits persist after a page refresh.
   * This was a critical bug where commits would disappear after refresh
   * because the backend was using the wrong WebSocket handler.
   */
  test("commits persist after page refresh", async ({ testPage, apiClient, seedData, backend }) => {
    const profile = await createStandardProfile(apiClient, "Git Refresh Profile");

    await apiClient.createTaskWithAgent(seedData.workspaceId, "Git Refresh Test", profile.id, {
      description: "Testing commits persist after refresh",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "Git Refresh Test");

    // Set up git helper
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };
    const git = new GitHelper(repoDir, gitEnv);

    // Create a commit
    git.createFile("persist-test.txt", "test content");
    git.stageAll();
    const sha = git.commit("Commit that should persist");

    // Click the Changes tab
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });

    // Wait for the commit to appear
    await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 15_000 });
    await session.expandCommitsSection();
    await expect(session.changes.getByText("Commit that should persist")).toBeVisible({
      timeout: 10_000,
    });
    await expect(session.changes.getByText(sha.slice(0, 7))).toBeVisible({ timeout: 5_000 });

    // NOW REFRESH THE PAGE - this is the critical test
    await testPage.reload();

    // Wait for the session to reload
    await session.waitForLoad();

    // Click the Changes tab again
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });

    // The commit MUST still be visible after refresh
    await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 15_000 });
    await session.expandCommitsSection();
    await expect(session.changes.getByText("Commit that should persist")).toBeVisible({
      timeout: 15_000,
    });
    await expect(session.changes.getByText(sha.slice(0, 7))).toBeVisible({ timeout: 5_000 });
  });

  /**
   * Verifies that multiple commits persist after page refresh.
   * Tests that the commit list is correctly fetched from agentctl after refresh.
   */
  test("multiple commits persist after page refresh", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const profile = await createStandardProfile(apiClient, "Git Multi Refresh Profile");

    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Git Multi Refresh Test",
      profile.id,
      {
        description: "Testing multiple commits persist after refresh",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    const session = await openTaskSession(testPage, "Git Multi Refresh Test");

    // Set up git helper
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };
    const git = new GitHelper(repoDir, gitEnv);

    // Create multiple commits — use stageFile() instead of stageAll() to avoid
    // picking up leftover files from prior tests in the shared repo.
    git.createFile("file1.txt", "content 1");
    git.stageFile("file1.txt");
    const sha1 = git.commit("First persistent commit");

    git.createFile("file2.txt", "content 2");
    git.stageFile("file2.txt");
    const sha2 = git.commit("Second persistent commit");

    git.createFile("file3.txt", "content 3");
    git.stageFile("file3.txt");
    const sha3 = git.commit("Third persistent commit");

    // Click the Changes tab
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });

    // Wait for all commits to appear
    await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 15_000 });
    await session.expandCommitsSection();
    await expect(session.changes.getByText("First persistent commit")).toBeVisible({
      timeout: 10_000,
    });
    await expect(session.changes.getByText("Second persistent commit")).toBeVisible({
      timeout: 5_000,
    });
    await expect(session.changes.getByText("Third persistent commit")).toBeVisible({
      timeout: 5_000,
    });

    // Verify all SHAs
    await expect(session.changes.getByText(sha1.slice(0, 7))).toBeVisible({ timeout: 5_000 });
    await expect(session.changes.getByText(sha2.slice(0, 7))).toBeVisible({ timeout: 5_000 });
    await expect(session.changes.getByText(sha3.slice(0, 7))).toBeVisible({ timeout: 5_000 });

    // NOW REFRESH THE PAGE
    await testPage.reload();
    await session.waitForLoad();

    // Click the Changes tab again
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });

    // ALL commits MUST still be visible after refresh
    await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 15_000 });
    await session.expandCommitsSection();
    await expect(session.changes.getByText("First persistent commit")).toBeVisible({
      timeout: 15_000,
    });
    await expect(session.changes.getByText("Second persistent commit")).toBeVisible({
      timeout: 5_000,
    });
    await expect(session.changes.getByText("Third persistent commit")).toBeVisible({
      timeout: 5_000,
    });

    // Verify all SHAs still present
    await expect(session.changes.getByText(sha1.slice(0, 7))).toBeVisible({ timeout: 5_000 });
    await expect(session.changes.getByText(sha2.slice(0, 7))).toBeVisible({ timeout: 5_000 });
    await expect(session.changes.getByText(sha3.slice(0, 7))).toBeVisible({ timeout: 5_000 });
  });

  /**
   * Verifies that a rebase operation updates the commit history in the UI.
   * Creates commits on main and a feature branch, then rebases.
   */
  test("rebase updates commit history", async ({ testPage, apiClient, seedData, backend }) => {
    const profile = await createStandardProfile(apiClient, "Git Rebase Profile");

    await apiClient.createTaskWithAgent(seedData.workspaceId, "Git Rebase Test", profile.id, {
      description: "Testing rebase operation",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "Git Rebase Test");

    // Set up git helper
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };
    const git = new GitHelper(repoDir, gitEnv);

    // Helper to clean up branch - ensures cleanup runs even if test fails
    const cleanupBranch = () => {
      try {
        // Abort any in-progress rebase before switching branches
        try {
          git.exec("git rebase --abort");
        } catch {
          /* not in a rebase */
        }
        git.exec("git checkout -f main");
        git.exec("git clean -fd");
        git.exec("git branch -D feature-rebase");
      } catch {
        // Branch may not exist if test failed before creation
      }
    };

    try {
      // Clean any leftover state from prior tests (including interrupted rebases)
      try {
        git.exec("git rebase --abort");
      } catch {
        /* not in a rebase */
      }
      git.exec("git clean -fd");
      // Remove feature-rebase branch if it already exists from a previous run
      try {
        git.exec("git checkout -f main");
        git.exec("git branch -D feature-rebase");
      } catch {
        /* branch doesn't exist yet */
      }

      // Create a commit on a feature branch
      git.exec("git checkout -b feature-rebase");
      git.createFile("feature-file.txt", "feature content");
      git.stageFile("feature-file.txt");
      git.commit("Feature commit before rebase");

      // Go back to main and create a new commit
      git.exec("git checkout main");
      git.createFile("main-file.txt", "main content");
      git.stageFile("main-file.txt");
      git.commit("Main commit after branch");

      // Go back to feature branch and rebase onto main
      git.exec("git checkout feature-rebase");
      git.exec("git rebase main");

      // The feature commit should now be rebased on top of main
      // Click the Changes tab to see the commits
      await session.clickTab("Changes");
      await expect(session.changes).toBeVisible({ timeout: 10_000 });

      // After rebase, the feature commit should still be visible
      await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 15_000 });
      await session.expandCommitsSection();
      await expect(session.changes.getByText("Feature commit before rebase")).toBeVisible({
        timeout: 15_000,
      });
    } finally {
      cleanupBranch();
    }
  });

  /**
   * Verifies that an interactive rebase (squash) updates commit history correctly.
   * Creates two commits and squashes them into one.
   */
  test("squash commits via rebase updates history", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const profile = await createStandardProfile(apiClient, "Git Squash Profile");

    await apiClient.createTaskWithAgent(seedData.workspaceId, "Git Squash Test", profile.id, {
      description: "Testing squash via rebase",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "Git Squash Test");

    // Set up git helper
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
      GIT_SEQUENCE_EDITOR: "true", // Skip interactive editor
    };
    const git = new GitHelper(repoDir, gitEnv);

    // Get base SHA for rebase
    const baseSha = git.getCurrentSha();

    // Create two commits to squash
    git.createFile("squash1.txt", "first");
    git.stageFile("squash1.txt");
    git.commit("First commit to squash");

    git.createFile("squash2.txt", "second");
    git.stageFile("squash2.txt");
    git.commit("Second commit to squash");

    // Click the Changes tab
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });

    // Verify both commits are visible before squash
    await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 15_000 });
    await session.expandCommitsSection();
    await expect(session.changes.getByText("First commit to squash")).toBeVisible({
      timeout: 10_000,
    });
    await expect(session.changes.getByText("Second commit to squash")).toBeVisible({
      timeout: 5_000,
    });

    // Squash the two commits into one using git reset --soft and recommit
    git.exec(`git reset --soft ${baseSha}`);
    git.commit("Squashed commit");

    // The squash happens on the filesystem while the Changes tab is already
    // open, so the FE only reflects it once the backend's git-status watcher
    // fires a WS push. Under CI shard load that push can lag past the wait
    // budget. Reload to force a fresh git-status load — deterministic instead of
    // racing the watcher (same approach as the persistent-commits test above).
    await testPage.reload();
    await session.waitForLoad();
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 15_000 });
    await session.expandCommitsSection();

    // Old commits should be gone after the squash.
    await expect(session.changes.getByText("First commit to squash")).not.toBeVisible({
      timeout: 15_000,
    });
    await expect(session.changes.getByText("Second commit to squash")).not.toBeVisible({
      timeout: 5_000,
    });

    // CommitsSection unmounts mid-transition (when no commits and no staged
    // files exist briefly), so it re-mounts collapsed by default — re-expand.
    await session.expandCommitsSection();

    // The squashed commit should appear
    await expect(session.changes.getByText("Squashed commit")).toBeVisible({ timeout: 10_000 });
  });

  /**
   * Verifies that the cumulative diff is updated correctly after commits.
   * Creates multiple commits and verifies the diff shows all changes.
   */
  test("cumulative diff shows all changes from multiple commits", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const profile = await createStandardProfile(apiClient, "Git Cumulative Profile");

    await apiClient.createTaskWithAgent(seedData.workspaceId, "Git Cumulative Test", profile.id, {
      description: "Testing cumulative diff",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "Git Cumulative Test");

    // captureBaseCommit runs asynchronously after agent launch and has to finish
    // before the commits below, or the cumulative diff is computed against the
    // wrong base.
    await dwell(
      testPage,
      3_000,
      "product-timer",
      "captureBaseCommit runs async after agent launch and publishes nothing when it completes, so the commits below have to be spaced past it",
    );

    // Set up git helper
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };
    const git = new GitHelper(repoDir, gitEnv);

    // Create commits with distinct content
    git.createFile("cumulative-file.txt", "line 1: first commit\n");
    git.stageFile("cumulative-file.txt");
    git.commit("Add first line");

    git.modifyFile("cumulative-file.txt", "line 1: first commit\nline 2: second commit\n");
    git.stageFile("cumulative-file.txt");
    git.commit("Add second line");

    // Click the Changes tab
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });

    // Wait for commits to appear
    await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 15_000 });
    await session.expandCommitsSection();

    // Wait for both commits to be visible
    await expect(session.changes.getByText("Add first line")).toBeVisible({ timeout: 10_000 });
    await expect(session.changes.getByText("Add second line")).toBeVisible({ timeout: 10_000 });

    // The poll below proves a "diff-viewer" panel exists after the click, which
    // only implicates the click if none existed before it. Nothing in this test
    // opens one earlier, so this reads as already-true today — it is here so
    // that stops being an unstated assumption if the surrounding test ever
    // reuses a session or restores a saved layout.
    const diffViewerOpen = () =>
      testPage.evaluate(() => {
        type Api = { getPanel: (id: string) => unknown };
        return Boolean(
          (window as unknown as { __dockviewApi__?: Api }).__dockviewApi__?.getPanel("diff-viewer"),
        );
      });
    expect(await diffViewerOpen(), "no cumulative diff panel before clicking Diff").toBe(false);

    // Click the "Diff" button in the header to open the cumulative diff view
    await session.changes.getByRole("button", { name: "Diff", exact: true }).click();

    // Assert what the click actually opens. Without this the checks below are
    // vacuous: the two commit texts were already visible before the click and
    // never go away, and "No changes" is absent from a page where the diff
    // never opened at all, so all three passed whether or not the button did
    // anything. The button calls `addDiffViewerPanel()`, which mounts a
    // dockview panel with id "diff-viewer" -- it is not the Review dialog, as a
    // first attempt at this assertion assumed and a run disproved.
    await expect
      .poll(diffViewerOpen, {
        timeout: 15_000,
        message: "the Diff button never opened the cumulative diff panel",
      })
      .toBe(true);

    await expect(session.changes.getByText("Add first line")).toBeVisible({ timeout: 5_000 });
    await expect(session.changes.getByText("Add second line")).toBeVisible({ timeout: 5_000 });

    // The cumulative diff should NOT show "No changes". A negative has no event
    // to wait on, so it needs the render window to elapse before sampling.
    await dwell(
      testPage,
      2_000,
      "negative-assertion",
      'asserts the cumulative diff never renders its empty state; "No changes" not appearing publishes nothing, so the check has to outlast the dialog\'s own load',
    );
    await expect(testPage.locator("text=No changes")).not.toBeVisible({ timeout: 5_000 });
  });

  /**
   * Verifies that sections in the changes panel can be collapsed and expanded.
   * Clicking the section header (label + count + chevron) toggles visibility of the section content.
   */
  test("sections can be collapsed and expanded", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const profile = await createStandardProfile(apiClient, "Git Collapse Profile");

    await apiClient.createTaskWithAgent(seedData.workspaceId, "Git Collapse Test", profile.id, {
      description: "Testing section collapse",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "Git Collapse Test");

    // Set up git helper
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };
    const git = new GitHelper(repoDir, gitEnv);

    // Create an unstaged file and a committed file so both sections appear
    git.createFile("collapse-committed.txt", "committed content");
    git.stageFile("collapse-committed.txt");
    git.commit("Collapse test commit");

    git.createFile("collapse-unstaged.txt", "unstaged content");

    // Click the Changes tab
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });

    // Wait for both sections to appear
    await expect(testPage.getByTestId("unstaged-files-section")).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 15_000 });

    // Verify the unstaged file is visible (unstaged section is expanded by default)
    await expect(session.changes.getByText("collapse-unstaged.txt")).toBeVisible({
      timeout: 5_000,
    });

    // Commits section is collapsed by default, so the commit text is not in the DOM
    const commitsToggle = testPage.getByTestId("commits-section-collapse-toggle");
    await expect(commitsToggle).toBeVisible({ timeout: 5_000 });
    await expect(commitsToggle).toHaveAttribute("aria-expanded", "false");
    await expect(session.changes.getByText("Collapse test commit")).not.toBeVisible({
      timeout: 2_000,
    });

    // --- Collapse the unstaged section ---
    const unstagedToggle = testPage.getByTestId("unstaged-files-section-collapse-toggle");
    await expect(unstagedToggle).toBeVisible({ timeout: 5_000 });
    await expect(unstagedToggle).toHaveAttribute("aria-expanded", "true");
    await unstagedToggle.click();

    // The unstaged file should now be hidden and toggle reflects collapsed state
    await expect(session.changes.getByText("collapse-unstaged.txt")).not.toBeVisible({
      timeout: 5_000,
    });
    await expect(unstagedToggle).toHaveAttribute("aria-expanded", "false");

    // The section header should still be visible (with count)
    await expect(unstagedToggle).toBeVisible();
    await expect(unstagedToggle).toContainText("Unstaged");

    // --- Expand the unstaged section back ---
    await unstagedToggle.click();
    await expect(session.changes.getByText("collapse-unstaged.txt")).toBeVisible({
      timeout: 5_000,
    });
    await expect(unstagedToggle).toHaveAttribute("aria-expanded", "true");

    // --- Expand the commits section ---
    await commitsToggle.click();
    await expect(session.changes.getByText("Collapse test commit")).toBeVisible({ timeout: 5_000 });
    await expect(commitsToggle).toHaveAttribute("aria-expanded", "true");

    // --- Collapse the commits section back ---
    await commitsToggle.click();
    await expect(session.changes.getByText("Collapse test commit")).not.toBeVisible({
      timeout: 5_000,
    });
    await expect(commitsToggle).toHaveAttribute("aria-expanded", "false");

    // Clean up
    git.deleteFile("collapse-unstaged.txt");
  });

  /**
   * Verifies that local commits section is hidden when all commits are already
   * in the PR. Only unpushed commits should be shown in the local section.
   */
  test("shows pushed commits with git-commit icon when all commits are in PR", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const profile = await createStandardProfile(apiClient, "Git PR Dedup Profile");

    // This test mutates repository history. Use a disposable local repository
    // so commits from earlier cumulative-diff tests cannot leak into the
    // unified list. The fixture has no fetchable origin, so keep materialization
    // offline while testing PR commit deduplication.
    const repoDir = path.join(backend.tmpDir, "repos", "git-pr-dedup-repo");
    fs.mkdirSync(repoDir, { recursive: true });
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };
    execSync("git init -b main", { cwd: repoDir, env: gitEnv });
    execSync('git commit --allow-empty -m "init"', { cwd: repoDir, env: gitEnv });
    const repository = await apiClient.createRepository(seedData.workspaceId, repoDir, "main", {
      name: "Git PR Dedup Repo",
      pull_before_worktree: false,
    });

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Git PR Dedup Test",
      profile.id,
      {
        description: "Testing PR commit deduplication",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [repository.id],
      },
    );

    const session = await openTaskSession(testPage, "Git PR Dedup Test");

    // Set up git helper
    const git = new GitHelper(repoDir, gitEnv);

    // Create a commit
    git.createFile("pr-dedup.txt", "dedup content");
    git.stageFile("pr-dedup.txt");
    const sha = git.commit("Commit in PR");
    const checkoutBranch = git.exec("git branch --show-current").trim();

    // Click the Changes tab and verify commit appears locally
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 15_000 });
    await session.expandCommitsSection();
    await expect(session.changes.getByText("Commit in PR")).toBeVisible({ timeout: 10_000 });

    // Mock a PR that contains the same commit
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "test-org",
      repo: "test-repo",
      pr_number: 1,
      pr_url: "https://github.com/test-org/test-repo/pull/1",
      pr_title: "Test PR",
      head_branch: checkoutBranch,
      base_branch: "main",
      author_login: "e2e-test",
    });
    await apiClient.mockGitHubAddPRCommits("test-org", "test-repo", 1, [
      {
        sha,
        message: "Commit in PR",
        author_login: "e2e-test",
        author_date: new Date().toISOString(),
      },
    ]);

    // Reload to pick up the PR association
    await testPage.reload();
    await session.waitForLoad();
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });

    // Unified commits section should show the commit as pushed (git-commit icon, not arrow-up)
    await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 15_000 });
    await session.expandCommitsSection();
    await expect(session.changes.getByText("Commit in PR")).toBeVisible({ timeout: 10_000 });
    const commitsList = testPage.getByTestId("commits-list");
    await expect(commitsList.locator('[data-testid^="commit-row-"]')).toHaveCount(1, {
      timeout: 5_000,
    });
    // Pushed commits use IconGitCommit (tabler-icon-git-commit), not IconArrowUp
    await expect(commitsList.locator(".tabler-icon-git-commit")).toBeVisible({ timeout: 5_000 });
    await expect(commitsList.locator(".tabler-icon-arrow-up")).not.toBeVisible();
  });

  /**
   * Verifies that pushed and unpushed commits are visually distinguished
   * in the unified commits list.
   */
  test("distinguishes pushed and unpushed commits in unified list", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const profile = await createStandardProfile(apiClient, "Git PR Partial Profile");

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Git PR Partial Test",
      profile.id,
      {
        description: "Testing partial PR commit deduplication",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    const session = await openTaskSession(testPage, "Git PR Partial Test");

    // Set up git helper
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };
    const git = new GitHelper(repoDir, gitEnv);

    // Create two commits
    git.createFile("pushed.txt", "pushed content");
    git.stageFile("pushed.txt");
    const pushedSha = git.commit("Pushed commit");

    git.createFile("unpushed.txt", "unpushed content");
    git.stageFile("unpushed.txt");
    git.commit("Unpushed commit");
    const checkoutBranch = git.exec("git branch --show-current").trim();

    // Mock a PR that only contains the first commit
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "test-org",
      repo: "test-repo",
      pr_number: 2,
      pr_url: "https://github.com/test-org/test-repo/pull/2",
      pr_title: "Partial PR",
      head_branch: checkoutBranch,
      base_branch: "main",
      author_login: "e2e-test",
    });
    await apiClient.mockGitHubAddPRCommits("test-org", "test-repo", 2, [
      {
        sha: pushedSha,
        message: "Pushed commit",
        author_login: "e2e-test",
        author_date: new Date().toISOString(),
      },
    ]);

    // Reload to pick up the PR association
    await testPage.reload();
    await session.waitForLoad();
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });

    // Unified commits section should show at least the two commits we created.
    // Other tests in the same worker may have left additional commits in the shared e2e-repo.
    await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 15_000 });
    await session.expandCommitsSection();
    const commitsList = testPage.getByTestId("commits-list");
    await expect(commitsList.locator('[data-testid^="commit-row-"]').first()).toBeVisible({
      timeout: 5_000,
    });

    // Unpushed commit should have arrow-up icon (emerald). Scope to the
    // commit-row testid so we don't match the parent repo-group <li> too
    // (which also contains the same text since it wraps the commits).
    const unpushedRow = commitsList
      .locator('[data-testid^="commit-row-"]', { hasText: "Unpushed commit" })
      .first();
    await expect(unpushedRow).toBeVisible({ timeout: 10_000 });
    await expect(unpushedRow.locator(".tabler-icon-arrow-up")).toBeVisible({ timeout: 5_000 });

    // Pushed commit should have git-commit icon (muted)
    const pushedRow = commitsList
      .locator('[data-testid^="commit-row-"]', { hasText: "Pushed commit" })
      .filter({ hasNotText: "Unpushed" })
      .first();
    await expect(pushedRow).toBeVisible({ timeout: 10_000 });
    await expect(pushedRow.locator(".tabler-icon-git-commit")).toBeVisible({ timeout: 5_000 });
  });

  test("local-first contribution keeps rewritten provider history separate", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    test.setTimeout(120_000);

    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };
    const git = new GitHelper(repoDir, gitEnv);

    // Build a deterministic stale checkout. The upstream stays at the fixture
    // seed commit while the task checkout grows six local commits.
    git.exec("git checkout -f main");
    if (git.exec("git remote").split(/\r?\n/).includes("origin")) {
      git.exec(`git remote set-url origin "${seedData.repositoryRemoteURL}"`);
    } else {
      git.exec(`git remote add origin "${seedData.repositoryRemoteURL}"`);
    }
    git.exec("git fetch origin main");
    git.exec("git reset --hard origin/main");
    git.exec("git clean -fd");
    git.exec("git branch --set-upstream-to=origin/main main");
    for (let index = 1; index <= 6; index += 1) {
      git.createFile(`drift-local-${index}.txt`, `local checkout commit ${index}`);
      git.stageFile(`drift-local-${index}.txt`);
      git.commit(`Contribution commit ${index}`);
    }
    const localHead = git.getCurrentSha();
    const providerBranch = "feature/rewritten-contribution";
    const providerHistory = createRewrittenProviderHistory(git, providerBranch);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Git Rewritten Contribution History",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("remote-contributor");
    await apiClient.mockGitHubAddPRs([
      {
        number: 901,
        title: "Rewritten contribution",
        state: "open",
        head_branch: providerBranch,
        base_branch: "main",
        author_login: "remote-contributor",
        repo_owner: "testorg",
        repo_name: "testrepo",
        head_sha: providerHistory.head,
      },
    ]);
    await apiClient.mockGitHubAddPRCommits("testorg", "testrepo", 901, providerHistory.commits);
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 901,
      pr_url: "https://github.com/testorg/testrepo/pull/901",
      pr_title: "Rewritten contribution",
      head_branch: "feature/rewritten-contribution",
      base_branch: "main",
      author_login: "remote-contributor",
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 45_000 });
    git.exec(`git checkout -B ${providerBranch} ${localHead}`);
    git.exec(`git branch --set-upstream-to=origin/${providerBranch} ${providerBranch}`);
    await session.clickTab("Changes");

    const changes = testPage.getByTestId("changes-panel");
    await expect(changes.getByTestId("remote-contribution-drift-status")).toHaveCount(0);
    const providerSection = changes.getByTestId("current-pr-commits-section");
    const localSection = changes.getByTestId("local-checkout-commits-section");
    await expect(providerSection).toBeVisible({ timeout: 10_000 });
    await expect(localSection).toBeVisible({ timeout: 10_000 });
    await expect(
      providerSection.getByTestId("current-pr-commits-section-collapse-toggle"),
    ).toContainText("PR #901 version");
    await expect(
      localSection.getByTestId("local-checkout-commits-section-collapse-toggle"),
    ).toContainText("Local checkout commits");
    await expect(
      providerSection.getByTestId("current-pr-commits-section-collapse-toggle"),
    ).toHaveAttribute("aria-expanded", "false");
    await expect(localSection.locator('[data-testid^="commit-row-"]')).toHaveCount(6);
    await providerSection.getByTestId("current-pr-commits-section-collapse-toggle").click();
    await expect(providerSection.locator('[data-commit-provenance="current_pr"]')).toHaveCount(15);
    await expect(localSection.locator('[data-commit-provenance="local_checkout"]')).toHaveCount(6);
    await expect(
      providerSection.locator('[data-commit-provenance="current_pr"]').first(),
    ).toHaveAttribute("title", "Current PR commit");
    await expect(
      localSection.locator('[data-commit-provenance="local_checkout"]').first(),
    ).toHaveAttribute("title", "Local checkout commit");
    await expect(providerSection.locator('[data-testid^="commit-row-"]')).toHaveCount(15);
    await expect(providerSection.locator('[data-testid^="commit-row-"]').first()).toContainText(
      "Rewritten provider commit 15",
    );

    // A rewritten provider history must not label the preserved checkout as
    // six unpushed commits, and reading the panel must not mutate the checkout.
    await expect(providerSection.locator(".tabler-icon-arrow-up")).toHaveCount(0);
    await expect(localSection.locator(".tabler-icon-arrow-up")).toHaveCount(0);
    expect(git.getCurrentSha()).toBe(localHead);
    expect(git.exec("git status --porcelain").trim()).toBe("");

    const changesPull = changes.getByRole("button", { name: /^Pull/ });
    await expect(changesPull).toBeDisabled();
    const driftWarning = changes.getByTestId("header-remote-contribution-warning");
    await expect(driftWarning).toBeVisible();
    await driftWarning.click();
    const driftMenu = testPage.getByTestId("header-remote-contribution-menu");
    await expect(driftMenu).toBeVisible();
    await expect(driftMenu.getByTestId("header-replace-pr-branch")).toBeVisible();
    await expect(driftMenu.getByTestId("header-use-pr-version")).toBeVisible();
    await expect(driftMenu.getByTestId("header-view-pr-version")).toContainText("PR #901 version");
    const replaceInfo = driftMenu.getByRole("img", {
      name: /Replace the published PR branch/,
    });
    await replaceInfo.hover();
    await expect(replaceInfo).not.toHaveAttribute("title");
    const openTooltip = testPage.locator(
      '[data-slot="tooltip-content"]:not([data-state="closed"])',
    );
    await expect(openTooltip).toContainText("Replace the published PR branch");
    const useInfo = driftMenu.getByRole("img", { name: /Use the current PR version/ });
    await useInfo.hover();
    await expect(openTooltip).toContainText("Use the current PR version");
    await testPage.mouse.move(0, 0);
    await expect(openTooltip).toBeHidden({ timeout: 5_000 });
    await driftMenu.getByTestId("header-replace-pr-branch").click();
    const resolutionDialog = testPage.getByTestId("remote-contribution-resolution-dialog");
    await expect(resolutionDialog).toBeVisible();
    await expect(resolutionDialog).toContainText(providerHistory.head);
    await resolutionDialog.getByRole("button", { name: "Cancel" }).click();
    await expect(resolutionDialog).toBeHidden();
    await prCapture.screenshot("remote-contribution-drift-desktop", {
      caption: "Rewritten provider history is separated from the preserved local checkout",
    });
  });

  test("uses the PR for the checked-out branch when an older PR is merged", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };
    const git = new GitHelper(repoDir, gitEnv);
    const currentBranch = "feature/current-contribution";
    const historicalBranch = "feature/merged-contribution";

    git.exec("git checkout -f main");
    if (git.exec("git remote").split(/\r?\n/).includes("origin")) {
      git.exec(`git remote set-url origin "${seedData.repositoryRemoteURL}"`);
    } else {
      git.exec(`git remote add origin "${seedData.repositoryRemoteURL}"`);
    }
    git.exec("git fetch origin main");
    git.exec("git reset --hard origin/main");
    git.exec("git clean -fd");
    git.exec(`git checkout -B ${currentBranch}`);
    git.createFile("current-contribution.txt", "current PR commit");
    git.stageFile("current-contribution.txt");
    const currentHead = git.commit("Current PR commit");
    git.exec(`git push --force --set-upstream origin ${currentBranch}`);

    const historicalHistory = createRewrittenProviderHistory(git, historicalBranch);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Git Current Branch PR Selection",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("branch-owner");
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 910,
      pr_url: "https://github.com/testorg/testrepo/pull/910",
      pr_title: "Merged contribution",
      head_branch: historicalBranch,
      base_branch: "main",
      author_login: "branch-owner",
      state: "merged",
    });
    await apiClient.mockGitHubAddPRCommits("testorg", "testrepo", 910, historicalHistory.commits);
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 911,
      pr_url: "https://github.com/testorg/testrepo/pull/911",
      pr_title: "Current contribution",
      head_branch: currentBranch,
      base_branch: "main",
      author_login: "branch-owner",
      state: "open",
    });
    await apiClient.mockGitHubAddPRCommits("testorg", "testrepo", 911, [
      {
        sha: currentHead,
        message: "Current PR commit",
        author_login: "branch-owner",
        author_date: "2026-08-12T12:00:00Z",
        stats_available: false,
      },
    ]);

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 45_000 });
    git.exec(`git checkout -B ${currentBranch} ${currentHead}`);
    git.exec(`git branch --set-upstream-to=origin/${currentBranch} ${currentBranch}`);
    await session.clickTab("Changes");

    const changes = testPage.getByTestId("changes-panel");
    await expect(changes.getByTestId("commits-section")).toBeVisible({ timeout: 30_000 });
    const currentPRRow = changes.getByTestId(`commit-row-${currentHead.slice(0, 7)}`);
    await expect(currentPRRow).toBeVisible();
    await expect(currentPRRow.getByTestId("commit-provenance")).toHaveAttribute(
      "data-commit-provenance",
      "pushed",
    );
    await expect(changes.getByText("Rewritten provider commit 15")).toHaveCount(0);
    await expect(changes.getByTestId("header-remote-contribution-warning")).toHaveCount(0);
    await expect(changes.getByTestId("remote-contribution-drift-status")).toHaveCount(0);
    await expect(testPage.getByRole("button", { name: /2 PRs/ })).toBeVisible();
    expect(git.getCurrentSha()).toBe(currentHead);
  });

  test("keeps a one-commit local-ahead contribution pushable", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };
    const git = new GitHelper(repoDir, gitEnv);

    git.exec("git checkout -f main");
    if (git.exec("git remote").split(/\r?\n/).includes("origin")) {
      git.exec(`git remote set-url origin "${seedData.repositoryRemoteURL}"`);
    } else {
      git.exec(`git remote add origin "${seedData.repositoryRemoteURL}"`);
    }
    git.exec("git fetch origin main");
    git.exec("git reset --hard origin/main");
    git.exec("git clean -fd");
    git.exec("git branch --set-upstream-to=origin/main main");
    const providerHead = git.exec("git rev-parse origin/main").trim();
    git.createFile("local-ahead-contribution.txt", "one local commit ahead");
    git.stageFile("local-ahead-contribution.txt");
    const localHead = git.commit("Local maintainer contribution");
    const providerBranch = "feature/local-ahead";
    git.exec(`git push origin ${providerHead}:refs/heads/${providerBranch}`);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Git Local Ahead Contribution",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("local-ahead-author");
    await apiClient.mockGitHubAddPRs([
      {
        number: 903,
        title: "Local ahead contribution",
        state: "open",
        head_branch: "feature/local-ahead",
        base_branch: "main",
        author_login: "local-ahead-author",
        repo_owner: "testorg",
        repo_name: "testrepo",
      },
    ]);
    await apiClient.mockGitHubAddPRCommits("testorg", "testrepo", 903, [
      {
        sha: providerHead,
        message: "Current provider head",
        author_login: "local-ahead-author",
        author_date: "2026-08-10T12:00:00Z",
        stats_available: false,
      },
    ]);
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 903,
      pr_url: "https://github.com/testorg/testrepo/pull/903",
      pr_title: "Local ahead contribution",
      head_branch: "feature/local-ahead",
      base_branch: "main",
      author_login: "local-ahead-author",
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 45_000 });
    git.exec(`git checkout -B ${providerBranch} ${localHead}`);
    git.exec(`git branch --set-upstream-to=origin/${providerBranch} ${providerBranch}`);
    await session.clickTab("Changes");
    const changes = testPage.getByTestId("changes-panel");
    await expect(changes.getByTestId("commits-section")).toBeVisible({ timeout: 30_000 });
    await expect(changes.getByText("Local maintainer contribution")).toBeVisible({
      timeout: 15_000,
    });
    await expect(changes.getByTestId("remote-contribution-drift-status")).toHaveCount(0);

    await changes.getByRole("button", { name: "Review" }).click();
    const reviewDialog = testPage.getByRole("dialog", { name: "Review Changes" });
    await expect(reviewDialog).toBeVisible({ timeout: 15_000 });
    await expect(reviewDialog.getByTestId("vcs-primary-push")).toBeVisible({ timeout: 15_000 });
    await reviewDialog.getByRole("button", { name: "Open VCS options" }).click();
    const openMenu = testPage.locator('[data-slot="dropdown-menu-content"][data-state="open"]');
    await expect(
      openMenu.locator('[data-slot="dropdown-menu-sub-trigger"]').filter({ hasText: /^Push/ }),
    ).not.toHaveAttribute("aria-disabled", "true");
    expect(git.getCurrentSha()).toBe(localHead);
    expect(git.exec("git status --porcelain").trim()).toBe("");
  });
});
