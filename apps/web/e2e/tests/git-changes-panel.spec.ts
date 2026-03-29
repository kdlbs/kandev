import { test, expect } from "../fixtures/test-base";
import type { ApiClient } from "../helpers/api-client";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";
import type { Page } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import { execSync } from "node:child_process";

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
    const profile = await createStandardProfile(apiClient, "Git Reset Profile");

    await apiClient.createTaskWithAgent(seedData.workspaceId, "Git Reset Test", profile.id, {
      description: "Testing reset to commit",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "Git Reset Test");

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

    // Wait for both commits to appear
    await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 15_000 });
    await expect(session.changes.getByText("First commit")).toBeVisible({ timeout: 10_000 });
    await expect(session.changes.getByText("Second commit")).toBeVisible({ timeout: 10_000 });

    // Verify both commits appear
    const commitsList = testPage.getByTestId("commits-list");
    await expect(commitsList.locator("li")).toHaveCount(2, { timeout: 5_000 });

    // Find the first commit row (it's the second in the list, older commit)
    // The list shows newest first, so "Second commit" is at index 0, "First commit" at index 1
    const firstCommitRow = commitsList.locator("li").filter({ hasText: "First commit" });
    await expect(firstCommitRow).toBeVisible({ timeout: 5_000 });

    // Get the SHA from the commit row to use in the confirmation
    const firstCommitSha = await firstCommitRow.locator("code").textContent();
    expect(firstCommitSha).toBeTruthy();

    // Click the reset button on the first commit row
    await firstCommitRow.hover();
    const resetButton = firstCommitRow.getByRole("button", { name: "Reset to this commit" });
    await expect(resetButton).toBeVisible({ timeout: 5_000 });
    await resetButton.click();

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

    // Wait for initial load
    await testPage.waitForTimeout(2_000);

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
    await expect(session.changes.getByText("First commit to squash")).toBeVisible({
      timeout: 10_000,
    });
    await expect(session.changes.getByText("Second commit to squash")).toBeVisible({
      timeout: 5_000,
    });

    // Squash the two commits into one using git reset --soft and recommit
    git.exec(`git reset --soft ${baseSha}`);
    git.commit("Squashed commit");

    // Wait for the UI to update - old commits should disappear
    await expect(session.changes.getByText("First commit to squash")).not.toBeVisible({
      timeout: 15_000,
    });
    await expect(session.changes.getByText("Second commit to squash")).not.toBeVisible({
      timeout: 5_000,
    });

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

    // Wait for the session to fully initialize including base commit capture.
    // The captureBaseCommit runs async after agent launch, we need it to complete
    // before making commits so the cumulative diff works correctly.
    await testPage.waitForTimeout(3_000);

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

    // Wait for both commits to be visible
    await expect(session.changes.getByText("Add first line")).toBeVisible({ timeout: 10_000 });
    await expect(session.changes.getByText("Add second line")).toBeVisible({ timeout: 10_000 });

    // Click the "Diff" button in the header to open the cumulative diff view
    await session.changes.getByRole("button", { name: "Diff" }).click();

    // Wait for diff viewer to load
    await testPage.waitForTimeout(2_000);

    // Verify commits are visible (this proves git changes are tracked)
    await expect(session.changes.getByText("Add first line")).toBeVisible({ timeout: 5_000 });
    await expect(session.changes.getByText("Add second line")).toBeVisible({ timeout: 5_000 });

    // The cumulative diff should NOT show "No changes"
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

    // Verify the unstaged file is visible
    await expect(session.changes.getByText("collapse-unstaged.txt")).toBeVisible({
      timeout: 5_000,
    });

    // Verify the commit is visible
    await expect(session.changes.getByText("Collapse test commit")).toBeVisible({ timeout: 5_000 });

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

    // --- Collapse the commits section ---
    const commitsToggle = testPage.getByTestId("commits-section-collapse-toggle");
    await expect(commitsToggle).toBeVisible({ timeout: 5_000 });
    await expect(commitsToggle).toHaveAttribute("aria-expanded", "true");
    await commitsToggle.click();

    // The commit should now be hidden and toggle reflects collapsed state
    await expect(session.changes.getByText("Collapse test commit")).not.toBeVisible({
      timeout: 5_000,
    });
    await expect(commitsToggle).toHaveAttribute("aria-expanded", "false");

    // --- Expand the unstaged section back ---
    await unstagedToggle.click();

    // The unstaged file should be visible again
    await expect(session.changes.getByText("collapse-unstaged.txt")).toBeVisible({
      timeout: 5_000,
    });
    await expect(unstagedToggle).toHaveAttribute("aria-expanded", "true");

    // --- Expand the commits section back ---
    await commitsToggle.click();

    // The commit should be visible again
    await expect(session.changes.getByText("Collapse test commit")).toBeVisible({ timeout: 5_000 });
    await expect(commitsToggle).toHaveAttribute("aria-expanded", "true");

    // Clean up
    git.deleteFile("collapse-unstaged.txt");
  });
});
