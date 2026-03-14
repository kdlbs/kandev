import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";

test.describe("Task creation from GitHub URL", () => {
  // Allow one retry for transient backend port-allocation issues on cold start.
  test.describe.configure({ retries: 1 });

  test("can create a task using a GitHub URL with workspace interaction", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);

    // Pre-seed a repository with GitHub provider info pointing to the real local git repo.
    // This lets FindOrCreateRepository find the repo (with its local_path) when the
    // GitHub URL is submitted, avoiding an actual clone.
    const repoDir = `${backend.tmpDir}/repos/e2e-repo`;
    await apiClient.createRepository(seedData.workspaceId, repoDir, "main", {
      name: "test-owner/test-repo",
      provider: "github",
      provider_owner: "test-owner",
      provider_name: "test-repo",
    });

    // Seed mock GitHub branches for the repo we'll reference
    await apiClient.mockGitHubAddBranches("test-owner", "test-repo", [
      { name: "main" },
      { name: "develop" },
      { name: "feature/test" },
    ]);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    // Toggle to GitHub URL mode
    const toggleBtn = testPage.getByTestId("toggle-github-url");
    await expect(toggleBtn).toBeVisible();
    await toggleBtn.click();

    // Enter a GitHub URL
    const urlInput = testPage.getByTestId("github-url-input");
    await expect(urlInput).toBeVisible();
    await urlInput.fill("https://github.com/test-owner/test-repo");

    // Fill in title and description
    await testPage.getByTestId("task-title-input").fill("GitHub URL Task");
    await testPage.getByTestId("task-description-input").fill("/e2e:simple-message");

    // Wait for the start button to become enabled (branches + agent profile resolved)
    const startBtn = testPage.getByTestId("submit-start-agent");
    await expect(startBtn).toBeEnabled({ timeout: 15_000 });

    // Click "Start task"
    await startBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    // The new task card appears on the kanban board
    const card = kanban.taskCardByTitle("GitHub URL Task");
    await expect(card).toBeVisible({ timeout: 10_000 });

    // Click the card to navigate to the session
    await card.click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Wait for the agent to complete
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });

    // Session transitions to idle
    await expect(session.idleInput()).toBeVisible({ timeout: 15_000 });

    // ── Workspace interaction: terminal + changes ──
    await expect(session.terminal).toBeVisible({ timeout: 15_000 });

    // Create a file via the terminal
    const testFileName = "ghtest";
    await session.typeInTerminal(`touch ${testFileName}`);

    // Switch to the Changes tab — the new file proves the terminal command worked
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });
    await expect(session.changes.getByText(testFileName)).toBeVisible({ timeout: 15_000 });
  });

  test("can create a GitHub URL task with worktree executor", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);

    // Pre-seed the GitHub-backed repository
    const repoDir = `${backend.tmpDir}/repos/e2e-repo`;
    await apiClient.createRepository(seedData.workspaceId, repoDir, "main", {
      name: "test-owner/test-repo",
      provider: "github",
      provider_owner: "test-owner",
      provider_name: "test-repo",
    });

    await apiClient.mockGitHubAddBranches("test-owner", "test-repo", [
      { name: "main" },
      { name: "develop" },
    ]);

    // Look up the worktree executor profile
    const { executors } = await apiClient.listExecutors();
    const worktreeExec = executors.find((e) => e.type === "worktree");
    const worktreeProfile = worktreeExec?.profiles?.[0];
    if (!worktreeProfile) {
      test.skip(true, "No worktree executor profile available");
      return;
    }

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    // Toggle to GitHub URL mode
    await testPage.getByTestId("toggle-github-url").click();
    await testPage.getByTestId("github-url-input").fill("https://github.com/test-owner/test-repo");

    // Fill in title and description
    await testPage.getByTestId("task-title-input").fill("Worktree GitHub Task");
    await testPage.getByTestId("task-description-input").fill("/e2e:simple-message");

    // Wait for selectors to be ready
    const startBtn = testPage.getByTestId("submit-start-agent");
    await expect(startBtn).toBeEnabled({ timeout: 15_000 });

    // Select the worktree executor profile
    const executorSelector = testPage.getByTestId("executor-profile-selector");
    await executorSelector.click();
    await testPage.getByRole("option", { name: /Worktree/i }).click();

    // Submit
    await startBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    const card = kanban.taskCardByTitle("Worktree GitHub Task");
    await expect(card).toBeVisible({ timeout: 10_000 });

    await card.click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });

    await expect(session.idleInput()).toBeVisible({ timeout: 15_000 });
  });

  test("shows error for invalid GitHub URL format", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();

    // Toggle to GitHub URL mode
    await testPage.getByTestId("toggle-github-url").click();
    const urlInput = testPage.getByTestId("github-url-input");
    const errorEl = testPage.getByTestId("github-url-error");

    // Type an invalid URL — error should appear
    await urlInput.fill("not-a-github-url");
    await expect(errorEl).toBeVisible({ timeout: 5_000 });
    await expect(errorEl).toContainText("Invalid GitHub URL");

    // Clear the input — error should disappear
    await urlInput.fill("");
    await expect(errorEl).not.toBeVisible({ timeout: 5_000 });

    // Type another invalid URL (missing repo)
    await urlInput.fill("https://github.com/owner-only");
    await expect(errorEl).toBeVisible({ timeout: 5_000 });
  });

  test("shows error for nonexistent repository", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();

    // Toggle to GitHub URL mode and enter a repo that isn't seeded in mock data
    await testPage.getByTestId("toggle-github-url").click();
    await testPage
      .getByTestId("github-url-input")
      .fill("https://github.com/no-such-owner/no-such-repo");

    // The error should appear after the branch fetch fails
    const errorEl = testPage.getByTestId("github-url-error");
    await expect(errorEl).toBeVisible({ timeout: 10_000 });
    await expect(errorEl).toContainText("not found or not accessible");
  });

  test("clears error when valid repository URL is entered", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    // Pre-seed a valid repo
    const repoDir = `${backend.tmpDir}/repos/e2e-repo`;
    await apiClient.createRepository(seedData.workspaceId, repoDir, "main", {
      name: "test-owner/test-repo",
      provider: "github",
      provider_owner: "test-owner",
      provider_name: "test-repo",
    });
    await apiClient.mockGitHubAddBranches("test-owner", "test-repo", [{ name: "main" }]);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();

    await testPage.getByTestId("toggle-github-url").click();
    const urlInput = testPage.getByTestId("github-url-input");
    const errorEl = testPage.getByTestId("github-url-error");

    // Start with an invalid URL — error appears
    await urlInput.fill("bad-url");
    await expect(errorEl).toBeVisible({ timeout: 5_000 });

    // Replace with a valid, seeded URL — error clears, branches load
    await urlInput.fill("https://github.com/test-owner/test-repo");
    await expect(errorEl).not.toBeVisible({ timeout: 10_000 });

    // Branches should load and start button should become enabled
    const startBtn = testPage.getByTestId("submit-start-agent");
    await testPage.getByTestId("task-title-input").fill("Validation Test");
    await testPage.getByTestId("task-description-input").fill("test");
    await expect(startBtn).toBeEnabled({ timeout: 15_000 });
  });

  test("uses correct repository when creating tasks from different GitHub URLs", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);

    // Seed two distinct GitHub-backed repositories
    const repoDirA = `${backend.tmpDir}/repos/e2e-repo`;
    await apiClient.createRepository(seedData.workspaceId, repoDirA, "main", {
      name: "owner-a/repo-a",
      provider: "github",
      provider_owner: "owner-a",
      provider_name: "repo-a",
    });
    await apiClient.mockGitHubAddBranches("owner-a", "repo-a", [{ name: "main" }]);

    const repoDirB = `${backend.tmpDir}/repos/e2e-repo-b`;
    const { execSync } = await import("child_process");
    const fs = await import("fs");
    fs.mkdirSync(repoDirB, { recursive: true });
    const gitEnv = {
      ...process.env,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };
    execSync("git init -b main", { cwd: repoDirB, env: gitEnv });
    execSync('git commit --allow-empty -m "init"', { cwd: repoDirB, env: gitEnv });
    await apiClient.createRepository(seedData.workspaceId, repoDirB, "main", {
      name: "owner-b/repo-b",
      provider: "github",
      provider_owner: "owner-b",
      provider_name: "repo-b",
    });
    await apiClient.mockGitHubAddBranches("owner-b", "repo-b", [{ name: "main" }]);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // ── First task from repo-a ──
    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    await testPage.getByTestId("toggle-github-url").click();
    await testPage.getByTestId("github-url-input").fill("https://github.com/owner-a/repo-a");
    await testPage.getByTestId("task-title-input").fill("Task A");
    await testPage.getByTestId("task-description-input").fill("/e2e:simple-message");
    const startBtn = testPage.getByTestId("submit-start-agent");
    await expect(startBtn).toBeEnabled({ timeout: 15_000 });
    await startBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });
    await expect(kanban.taskCardByTitle("Task A")).toBeVisible({ timeout: 10_000 });

    // ── Second task from repo-b (different URL) ──
    await kanban.createTaskButton.first().click();
    await expect(dialog).toBeVisible();
    await testPage.getByTestId("toggle-github-url").click();
    await testPage.getByTestId("github-url-input").fill("https://github.com/owner-b/repo-b");
    await testPage.getByTestId("task-title-input").fill("Task B");
    await testPage.getByTestId("task-description-input").fill("/e2e:simple-message");
    await expect(startBtn).toBeEnabled({ timeout: 15_000 });
    await startBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    // Both cards visible — second task created successfully on its own repo
    await expect(kanban.taskCardByTitle("Task B")).toBeVisible({ timeout: 10_000 });

    // Navigate to Task B session and verify agent completes
    await kanban.taskCardByTitle("Task B").click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });
  });

  test("PR URL auto-selects the PR head branch", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    // Pre-seed a GitHub-backed repository
    const repoDir = `${backend.tmpDir}/repos/e2e-repo`;
    await apiClient.createRepository(seedData.workspaceId, repoDir, "main", {
      name: "test-owner/test-repo",
      provider: "github",
      provider_owner: "test-owner",
      provider_name: "test-repo",
    });

    // Seed branches including the PR head branch
    await apiClient.mockGitHubAddBranches("test-owner", "test-repo", [
      { name: "main" },
      { name: "feature/pr-branch" },
    ]);

    // Seed a PR pointing to the feature branch
    await apiClient.mockGitHubAddPRs([
      {
        number: 42,
        title: "Test PR",
        state: "open",
        head_branch: "feature/pr-branch",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "test-owner",
        repo_name: "test-repo",
      },
    ]);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    // Toggle to GitHub URL mode and enter a PR URL
    await testPage.getByTestId("toggle-github-url").click();
    await testPage
      .getByTestId("github-url-input")
      .fill("https://github.com/test-owner/test-repo/pull/42");

    // The PR head branch should be auto-selected in the branch selector
    await expect(dialog.getByText("feature/pr-branch")).toBeVisible({ timeout: 10_000 });
  });

  test("creates task from PR URL with local executor end-to-end", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);

    const { execSync } = await import("child_process");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };

    // Create the feature/pr-branch in the local repo so git checkout works
    const repoDir = `${backend.tmpDir}/repos/e2e-repo`;
    execSync("git checkout -b feature/pr-branch", { cwd: repoDir, env: gitEnv });
    execSync('git commit --allow-empty -m "pr commit"', { cwd: repoDir, env: gitEnv });
    execSync("git checkout main", { cwd: repoDir, env: gitEnv });

    // Pre-seed a GitHub-backed repository
    await apiClient.createRepository(seedData.workspaceId, repoDir, "main", {
      name: "test-owner/test-repo",
      provider: "github",
      provider_owner: "test-owner",
      provider_name: "test-repo",
    });

    await apiClient.mockGitHubAddBranches("test-owner", "test-repo", [
      { name: "main" },
      { name: "feature/pr-branch" },
    ]);

    await apiClient.mockGitHubAddPRs([
      {
        number: 99,
        title: "Feature PR",
        state: "open",
        head_branch: "feature/pr-branch",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "test-owner",
        repo_name: "test-repo",
      },
    ]);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    // Toggle to GitHub URL mode and enter the PR URL
    await testPage.getByTestId("toggle-github-url").click();
    await testPage
      .getByTestId("github-url-input")
      .fill("https://github.com/test-owner/test-repo/pull/99");

    // Verify PR head branch is auto-selected
    await expect(dialog.getByText("feature/pr-branch")).toBeVisible({ timeout: 10_000 });

    // Fill in title and description
    await testPage.getByTestId("task-title-input").fill("PR Task Local");
    await testPage.getByTestId("task-description-input").fill("/e2e:simple-message");

    // Wait for the start button to become enabled
    const startBtn = testPage.getByTestId("submit-start-agent");
    await expect(startBtn).toBeEnabled({ timeout: 15_000 });

    // Submit (default executor is local/standalone)
    await startBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    // The new task card appears on the kanban board
    const card = kanban.taskCardByTitle("PR Task Local");
    await expect(card).toBeVisible({ timeout: 10_000 });

    // Navigate to the session and verify the agent completes
    await card.click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });

    await expect(session.idleInput()).toBeVisible({ timeout: 15_000 });

    // Verify the local executor checked out the PR branch.
    await expect(session.terminal).toBeVisible({ timeout: 15_000 });
    await session.typeInTerminal("git branch --show-current");
    await session.expectTerminalHasText("feature/pr-branch");
  });

  test("creates task from PR URL with worktree executor and verifies branch", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);

    const { execSync } = await import("child_process");
    const fs = await import("fs");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };

    // Create a fresh repo with the PR branch available locally.
    // No remote needed — the worktree manager falls back to local branches.
    const repoDir = `${backend.tmpDir}/repos/e2e-pr-wt`;
    fs.mkdirSync(repoDir, { recursive: true });

    execSync("git init -b main", { cwd: repoDir, env: gitEnv });
    execSync('git commit --allow-empty -m "init"', { cwd: repoDir, env: gitEnv });

    // Create the PR branch locally and switch back to main
    execSync("git checkout -b feature/pr-branch", { cwd: repoDir, env: gitEnv });
    execSync('git commit --allow-empty -m "pr branch commit"', { cwd: repoDir, env: gitEnv });
    execSync("git checkout main", { cwd: repoDir, env: gitEnv });

    // Register the repo with a unique provider name to avoid collisions with
    // other tests that also register repos as test-owner/test-repo.
    await apiClient.createRepository(seedData.workspaceId, repoDir, "main", {
      name: "pr-owner/pr-wt-repo",
      provider: "github",
      provider_owner: "pr-owner",
      provider_name: "pr-wt-repo",
    });

    // Seed mock GitHub branches and PR
    await apiClient.mockGitHubAddBranches("pr-owner", "pr-wt-repo", [
      { name: "main" },
      { name: "feature/pr-branch" },
    ]);

    await apiClient.mockGitHubAddPRs([
      {
        number: 77,
        title: "Worktree PR",
        state: "open",
        head_branch: "feature/pr-branch",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "pr-owner",
        repo_name: "pr-wt-repo",
      },
    ]);

    // Look up the worktree executor profile
    const { executors } = await apiClient.listExecutors();
    const worktreeExec = executors.find((e) => e.type === "worktree");
    const worktreeProfile = worktreeExec?.profiles?.[0];
    if (!worktreeProfile) {
      test.skip(true, "No worktree executor profile available");
      return;
    }

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    // Toggle to GitHub URL mode and enter the PR URL
    await testPage.getByTestId("toggle-github-url").click();
    await testPage
      .getByTestId("github-url-input")
      .fill("https://github.com/pr-owner/pr-wt-repo/pull/77");

    // Verify PR head branch is auto-selected
    await expect(dialog.getByText("feature/pr-branch")).toBeVisible({ timeout: 10_000 });

    // Fill in title and description
    await testPage.getByTestId("task-title-input").fill("PR Worktree Task");
    await testPage.getByTestId("task-description-input").fill("/e2e:simple-message");

    // Wait for selectors to be ready
    const startBtn = testPage.getByTestId("submit-start-agent");
    await expect(startBtn).toBeEnabled({ timeout: 15_000 });

    // Select the worktree executor profile
    const executorSelector = testPage.getByTestId("executor-profile-selector");
    await executorSelector.click();
    await testPage.getByRole("option", { name: /Worktree/i }).click();

    // Submit
    await startBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    const card = kanban.taskCardByTitle("PR Worktree Task");
    await expect(card).toBeVisible({ timeout: 10_000 });

    await card.click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });

    await expect(session.idleInput()).toBeVisible({ timeout: 15_000 });

    // Verify the worktree checked out the PR branch directly.
    await expect(session.terminal).toBeVisible({ timeout: 15_000 });
    await session.typeInTerminal("git branch --show-current");
    await session.expectTerminalHasText("feature/pr-branch");
  });

  test("shows fetch warning banner when PR branch has no remote", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);

    const { execSync } = await import("child_process");
    const fs = await import("fs");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };

    // Create a repo with the PR branch locally but NO remote.
    // This causes fetchBranchToLocal to fail the fetch and fall back to local,
    // which produces a warning that should be displayed in the UI.
    const repoDir = `${backend.tmpDir}/repos/e2e-warning-repo`;
    fs.mkdirSync(repoDir, { recursive: true });
    execSync("git init -b main", { cwd: repoDir, env: gitEnv });
    execSync('git commit --allow-empty -m "init"', { cwd: repoDir, env: gitEnv });
    execSync("git checkout -b feature/warn-branch", { cwd: repoDir, env: gitEnv });
    execSync('git commit --allow-empty -m "feature commit"', { cwd: repoDir, env: gitEnv });
    execSync("git checkout main", { cwd: repoDir, env: gitEnv });

    await apiClient.createRepository(seedData.workspaceId, repoDir, "main", {
      name: "warn-owner/warn-repo",
      provider: "github",
      provider_owner: "warn-owner",
      provider_name: "warn-repo",
    });

    await apiClient.mockGitHubAddBranches("warn-owner", "warn-repo", [
      { name: "main" },
      { name: "feature/warn-branch" },
    ]);

    await apiClient.mockGitHubAddPRs([
      {
        number: 200,
        title: "Warning PR",
        state: "open",
        head_branch: "feature/warn-branch",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "warn-owner",
        repo_name: "warn-repo",
      },
    ]);

    // Need worktree executor to trigger fetchBranchToLocal (local executor doesn't produce warnings)
    const { executors } = await apiClient.listExecutors();
    const worktreeExec = executors.find((e) => e.type === "worktree");
    const worktreeProfile = worktreeExec?.profiles?.[0];
    if (!worktreeProfile) {
      test.skip(true, "No worktree executor profile available");
      return;
    }

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    // Enter PR URL
    await testPage.getByTestId("toggle-github-url").click();
    await testPage
      .getByTestId("github-url-input")
      .fill("https://github.com/warn-owner/warn-repo/pull/200");

    await expect(dialog.getByText("feature/warn-branch")).toBeVisible({ timeout: 10_000 });

    await testPage.getByTestId("task-title-input").fill("Warning Test Task");
    await testPage.getByTestId("task-description-input").fill("/e2e:simple-message");

    const startBtn = testPage.getByTestId("submit-start-agent");
    await expect(startBtn).toBeEnabled({ timeout: 15_000 });

    // Select worktree executor
    const executorSelector = testPage.getByTestId("executor-profile-selector");
    await executorSelector.click();
    await testPage.getByRole("option", { name: /Worktree/i }).click();

    await startBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    const card = kanban.taskCardByTitle("Warning Test Task");
    await expect(card).toBeVisible({ timeout: 10_000 });

    await card.click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Wait for agent to complete (preparation is done by this point)
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });

    // The warning banner should persist after preparation completes.
    // It shows because fetchBranchToLocal failed to reach origin (no remote)
    // and fell back to the local branch.
    const warningBanner = testPage.getByTestId("prepare-warning-banner");
    await expect(warningBanner).toBeVisible({ timeout: 10_000 });
    await expect(warningBanner).toContainText("Could not fetch latest from origin");

    // The "Details" toggle should be visible and expand raw git output on click.
    const detailsBtn = warningBanner.getByRole("button", { name: "Details" });
    await expect(detailsBtn).toBeVisible();
    await detailsBtn.click();
    // After expanding, raw git output should appear (e.g. "fatal:" from the failed fetch)
    await expect(warningBanner.locator("pre")).toBeVisible();

    // Clicking again should collapse the details.
    await detailsBtn.click();
    await expect(warningBanner.locator("pre")).not.toBeVisible();

    // Dismissing the banner hides it entirely.
    const dismissBtn = warningBanner.getByRole("button", { name: "Dismiss warning" });
    await dismissBtn.click();
    await expect(warningBanner).not.toBeVisible();
  });

  test("two tasks from the same PR URL create independent worktrees", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    const { execSync } = await import("child_process");
    const fs = await import("fs");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };

    // Create a repo with the PR branch locally.
    const repoDir = `${backend.tmpDir}/repos/e2e-shared-pr`;
    fs.mkdirSync(repoDir, { recursive: true });
    execSync("git init -b main", { cwd: repoDir, env: gitEnv });
    execSync('git commit --allow-empty -m "init"', { cwd: repoDir, env: gitEnv });
    execSync("git checkout -b feature/shared-pr", { cwd: repoDir, env: gitEnv });
    execSync('git commit --allow-empty -m "shared pr commit"', { cwd: repoDir, env: gitEnv });
    execSync("git checkout main", { cwd: repoDir, env: gitEnv });

    await apiClient.createRepository(seedData.workspaceId, repoDir, "main", {
      name: "shared-owner/shared-repo",
      provider: "github",
      provider_owner: "shared-owner",
      provider_name: "shared-repo",
    });

    await apiClient.mockGitHubAddBranches("shared-owner", "shared-repo", [
      { name: "main" },
      { name: "feature/shared-pr" },
    ]);

    await apiClient.mockGitHubAddPRs([
      {
        number: 50,
        title: "Shared PR",
        state: "open",
        head_branch: "feature/shared-pr",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "shared-owner",
        repo_name: "shared-repo",
      },
    ]);

    // Look up the worktree executor profile
    const { executors } = await apiClient.listExecutors();
    const worktreeExec = executors.find((e) => e.type === "worktree");
    const worktreeProfile = worktreeExec?.profiles?.[0];
    if (!worktreeProfile) {
      test.skip(true, "No worktree executor profile available");
      return;
    }

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // Helper: select worktree executor in the create dialog
    const selectWorktreeExecutor = async () => {
      const selector = testPage.getByTestId("executor-profile-selector");
      // Only click if the selector doesn't already show "Worktree"
      const selectorText = await selector.textContent();
      if (!selectorText?.includes("Worktree")) {
        await selector.click();
        await testPage.getByRole("option", { name: /Worktree/i }).click();
      }
    };

    // --- Task A: first task from PR URL ---
    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    await testPage.getByTestId("toggle-github-url").click();
    await testPage
      .getByTestId("github-url-input")
      .fill("https://github.com/shared-owner/shared-repo/pull/50");
    await expect(dialog.getByText("feature/shared-pr")).toBeVisible({ timeout: 10_000 });

    await testPage.getByTestId("task-title-input").fill("Shared PR Task A");
    await testPage.getByTestId("task-description-input").fill("/e2e:simple-message");

    const startBtn = testPage.getByTestId("submit-start-agent");
    await expect(startBtn).toBeEnabled({ timeout: 15_000 });
    await selectWorktreeExecutor();

    await startBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });
    await expect(kanban.taskCardByTitle("Shared PR Task A")).toBeVisible({ timeout: 10_000 });

    // Wait for Task A agent to complete before creating Task B
    await kanban.taskCardByTitle("Shared PR Task A").click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });
    const sessionA = new SessionPage(testPage);
    await sessionA.waitForLoad();
    await expect(sessionA.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });
    await expect(sessionA.idleInput()).toBeVisible({ timeout: 15_000 });

    // Task A should have the direct PR branch
    await expect(sessionA.terminal).toBeVisible({ timeout: 15_000 });
    await sessionA.typeInTerminal("git branch --show-current");
    await sessionA.expectTerminalHasText("feature/shared-pr");

    // --- Task B: second task from the same PR URL ---
    await testPage.goto("/");
    await kanban.board.waitFor({ state: "visible" });

    await kanban.createTaskButton.first().click();
    await expect(dialog).toBeVisible();

    await testPage.getByTestId("toggle-github-url").click();
    await testPage
      .getByTestId("github-url-input")
      .fill("https://github.com/shared-owner/shared-repo/pull/50");
    await expect(dialog.getByText("feature/shared-pr")).toBeVisible({ timeout: 10_000 });

    await testPage.getByTestId("task-title-input").fill("Shared PR Task B");
    await testPage.getByTestId("task-description-input").fill("/e2e:simple-message");

    await expect(startBtn).toBeEnabled({ timeout: 15_000 });
    await selectWorktreeExecutor();

    await startBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    // Task B should also succeed (this would have failed before the fix)
    await expect(kanban.taskCardByTitle("Shared PR Task B")).toBeVisible({ timeout: 10_000 });

    await kanban.taskCardByTitle("Shared PR Task B").click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });
    const sessionB = new SessionPage(testPage);
    await sessionB.waitForLoad();
    await expect(sessionB.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });

    // Task B should have a suffixed branch (not the original PR branch)
    await expect(sessionB.terminal).toBeVisible({ timeout: 15_000 });
    await sessionB.typeInTerminal("git branch --show-current");
    // Branch should start with the PR branch name but have a random suffix
    await sessionB.expectTerminalHasText("feature/shared-pr-");
  });

  test("can toggle between GitHub URL and repository selector", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    // Initially the toggle says "or paste a GitHub URL"
    const toggleBtn = testPage.getByTestId("toggle-github-url");
    await expect(toggleBtn).toHaveText("or paste a GitHub URL");

    // Toggle to GitHub URL mode
    await toggleBtn.click();
    await expect(testPage.getByTestId("github-url-input")).toBeVisible();
    await expect(toggleBtn).toHaveText("or select a repository");

    // Toggle back to repository selector
    await toggleBtn.click();
    await expect(testPage.getByTestId("github-url-input")).not.toBeVisible();
    await expect(toggleBtn).toHaveText("or paste a GitHub URL");
  });
});
