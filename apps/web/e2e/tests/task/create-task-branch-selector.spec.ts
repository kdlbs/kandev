import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

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
    const profile = await apiClient.createExecutorProfile(localExec.id, "E2E Local Profile");

    try {
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

      // Branch selector should be disabled when local executor is selected
      const branchSelector = testPage.getByTestId("branch-selector");
      await expect(branchSelector).toBeDisabled({ timeout: 5_000 });
    } finally {
      await apiClient.deleteExecutorProfile(profile.id).catch(() => {});
    }
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
    const profile = await apiClient.createExecutorProfile(localExec.id, "E2E Local");
    const worktreeProfile = worktreeExec.profiles?.[0];
    if (!worktreeProfile) {
      test.skip(true, "No worktree profile available");
      return;
    }

    try {
      const kanban = new KanbanPage(testPage);
      await kanban.goto();

      await kanban.createTaskButton.first().click();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();

      await testPage.getByTestId("task-title-input").fill("Switch Executor Test");
      await testPage.getByTestId("task-description-input").fill("testing executor switch");

      // Select local executor -> branch should be disabled
      const executorSelector = testPage.getByTestId("executor-profile-selector");
      await executorSelector.click();
      await testPage.getByRole("option", { name: /E2E Local/i }).click();

      const branchSelector = testPage.getByTestId("branch-selector");
      await expect(branchSelector).toBeDisabled({ timeout: 5_000 });

      // Switch to worktree executor -> branch should be enabled
      await executorSelector.click();
      await testPage.getByRole("option", { name: worktreeProfile.name }).click();

      await expect(branchSelector).toBeEnabled({ timeout: 5_000 });
    } finally {
      await apiClient.deleteExecutorProfile(profile.id).catch(() => {});
    }
  });

  test("branch selector stays enabled for local executor with GitHub URL", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const fs = await import("fs");
    const { execSync } = await import("child_process");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };

    // Find local executor and create profile
    const { executors } = await apiClient.listExecutors();
    const localExec = executors.find((e) => e.type === "local");
    if (!localExec) {
      test.skip(true, "No local executor available");
      return;
    }
    const profile = await apiClient.createExecutorProfile(localExec.id, "E2E Local GitHub URL");

    try {
      // Create a unique repo for this test
      const repoDir = `${backend.tmpDir}/repos/e2e-branch-gh`;
      fs.mkdirSync(repoDir, { recursive: true });
      execSync("git init -b main", { cwd: repoDir, env: gitEnv });
      execSync('git commit --allow-empty -m "init"', { cwd: repoDir, env: gitEnv });

      // Seed mock GitHub branches
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
      await expect(branchSelector).toBeEnabled({ timeout: 10_000 });
    } finally {
      await apiClient.deleteExecutorProfile(profile.id).catch(() => {});
    }
  });
});
