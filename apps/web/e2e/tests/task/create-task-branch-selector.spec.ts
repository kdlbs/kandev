import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

const START_AGENT_TEST_ID = "submit-start-agent";
const START_ENABLED_TIMEOUT = 30_000;

test.describe("Branch selector behavior with executor types", () => {
  test.describe.configure({ retries: 1 });

  test("branch selector is disabled for local executor with local repo", async ({
    testPage,
    apiClient,
  }) => {
    // Find the system "local" executor and create a profile on it
    const { executors } = await apiClient.listExecutors();
    const localExec = executors.find((e) => e.type === "local");
    if (!localExec) {
      test.skip(true, "No local executor available");
      return;
    }
    const localProfile = await apiClient.createExecutorProfile(localExec.id, "E2E Local Profile");

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    // Fill title + description
    await testPage.getByTestId("task-title-input").fill("Branch Selector Test");
    await testPage.getByTestId("task-description-input").fill("testing branch selector disable");

    // Select the local executor profile
    const executorSelector = testPage.getByTestId("executor-profile-selector");
    await executorSelector.click();
    await testPage.getByRole("option", { name: /E2E Local Profile/i }).click();

    // Branch selector should be disabled and show "Uses current branch"
    const branchSelector = testPage.getByTestId("branch-selector");
    await expect(branchSelector.locator("button")).toBeDisabled({ timeout: 5_000 });
    await expect(branchSelector).toContainText("Uses current branch");

    // Cleanup
    await apiClient.deleteExecutorProfile(localProfile.id);
  });

  test("branch selector re-enables when switching from local to worktree executor", async ({
    testPage,
    apiClient,
  }) => {
    // Find executors
    const { executors } = await apiClient.listExecutors();
    const localExec = executors.find((e) => e.type === "local");
    const worktreeExec = executors.find((e) => e.type === "worktree");
    if (!localExec || !worktreeExec) {
      test.skip(true, "Need both local and worktree executors");
      return;
    }
    const localProfile = await apiClient.createExecutorProfile(localExec.id, "E2E Local");
    const worktreeProfile = worktreeExec.profiles?.[0];
    if (!worktreeProfile) {
      test.skip(true, "No worktree profile available");
      return;
    }

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    await testPage.getByTestId("task-title-input").fill("Switch Executor Test");
    await testPage.getByTestId("task-description-input").fill("testing executor switch");

    // Select local executor → branch should be disabled
    const executorSelector = testPage.getByTestId("executor-profile-selector");
    await executorSelector.click();
    await testPage.getByRole("option", { name: /E2E Local/i }).click();

    const branchSelector = testPage.getByTestId("branch-selector");
    await expect(branchSelector.locator("button")).toBeDisabled({ timeout: 5_000 });

    // Switch to worktree executor → branch should be enabled
    await executorSelector.click();
    await testPage.getByRole("option", { name: new RegExp(worktreeProfile.name, "i") }).click();

    await expect(branchSelector.locator("button")).toBeEnabled({ timeout: 5_000 });

    // Cleanup
    await apiClient.deleteExecutorProfile(localProfile.id);
  });

  test("branch selector stays enabled for local executor with GitHub URL", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    // Find local executor and create profile
    const { executors } = await apiClient.listExecutors();
    const localExec = executors.find((e) => e.type === "local");
    if (!localExec) {
      test.skip(true, "No local executor available");
      return;
    }
    const localProfile = await apiClient.createExecutorProfile(
      localExec.id,
      "E2E Local GitHub URL",
    );

    // Seed mock GitHub branches
    const repoDir = `${backend.tmpDir}/repos/e2e-repo`;
    await apiClient.createRepository(seedData.workspaceId, repoDir, "main", {
      name: "branch-test-owner/branch-test-repo",
      provider: "github",
      provider_owner: "branch-test-owner",
      provider_name: "branch-test-repo",
    });
    await apiClient.mockGitHubAddBranches("branch-test-owner", "branch-test-repo", [
      { name: "main" },
      { name: "develop" },
    ]);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    // Toggle to GitHub URL mode
    await testPage.getByTestId("toggle-github-url").click();
    await testPage
      .getByTestId("github-url-input")
      .fill("https://github.com/branch-test-owner/branch-test-repo");

    // Select local executor profile
    const executorSelector = testPage.getByTestId("executor-profile-selector");
    await executorSelector.click();
    await testPage.getByRole("option", { name: /E2E Local GitHub URL/i }).click();

    // Branch selector should NOT be disabled (GitHub URL mode overrides)
    const branchSelector = testPage.getByTestId("branch-selector");
    await expect(branchSelector.locator("button")).toBeEnabled({ timeout: 10_000 });

    // Cleanup
    await apiClient.deleteExecutorProfile(localProfile.id);
  });

  test("GitHub URL base branch checkout works with local executor", async ({
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

    // Create a repo with a develop branch
    const repoDir = `${backend.tmpDir}/repos/e2e-repo`;
    execSync("git checkout -b develop", { cwd: repoDir, env: gitEnv });
    execSync('git commit --allow-empty -m "develop commit"', { cwd: repoDir, env: gitEnv });
    execSync("git checkout main", { cwd: repoDir, env: gitEnv });

    // Pre-seed GitHub-backed repository
    await apiClient.createRepository(seedData.workspaceId, repoDir, "main", {
      name: "base-branch-owner/base-branch-repo",
      provider: "github",
      provider_owner: "base-branch-owner",
      provider_name: "base-branch-repo",
    });

    await apiClient.mockGitHubAddBranches("base-branch-owner", "base-branch-repo", [
      { name: "main" },
      { name: "develop" },
    ]);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    // Toggle to GitHub URL mode and enter URL
    await testPage.getByTestId("toggle-github-url").click();
    await testPage
      .getByTestId("github-url-input")
      .fill("https://github.com/base-branch-owner/base-branch-repo");

    // Wait for branches to load and select "develop"
    const branchSelector = testPage.getByTestId("branch-selector");
    await expect(branchSelector.locator("button")).toBeEnabled({ timeout: 10_000 });
    await branchSelector.click();
    await testPage.getByRole("option", { name: "develop" }).click();

    // Fill title and description
    await testPage.getByTestId("task-title-input").fill("Base Branch Checkout Test");
    await testPage.getByTestId("task-description-input").fill("/e2e:simple-message");

    // Start the task
    const startBtn = testPage.getByTestId(START_AGENT_TEST_ID);
    await expect(startBtn).toBeEnabled({ timeout: START_ENABLED_TIMEOUT });
    await startBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    // Navigate to session
    const card = kanban.taskCardByTitle("Base Branch Checkout Test");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });

    // Verify the local executor checked out the develop branch
    await expect(session.terminal).toBeVisible({ timeout: 15_000 });
    await session.typeInTerminal("git branch --show-current");
    await session.expectTerminalHasText("develop");
  });
});
