import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";

test.describe("Task creation from GitHub URL", () => {
  test("can create a task using a GitHub URL", async ({ testPage, apiClient }) => {
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
