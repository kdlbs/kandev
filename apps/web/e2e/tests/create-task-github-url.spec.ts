import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";

test.describe("Task creation from GitHub URL", () => {
  test("can create a task using a GitHub URL", async ({ testPage, apiClient, seedData, backend }) => {
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
  });

  test("can create a GitHub URL task with worktree executor", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
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
