import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";

test.describe("PR watcher dockview layout stability", () => {
  /**
   * Verifies the dockview layout stays correct when switching between
   * PR tasks auto-created by the review watcher.
   *
   * Flow:
   *   1. PR watcher detects 3 PRs and auto-creates tasks in the background
   *   2. Open PR task 1 from the kanban homepage
   *   3. Toggle plan mode (adds plan panel)
   *   4. Switch to PR task 2 via sidebar → should have default layout
   *   5. Switch to PR task 3 via sidebar → should have default layout
   *
   * Setup:
   *   Review step (no auto_start — tasks sit idle after creation)
   *   Review watch triggers initial poll → 3 tasks created (not started)
   */
  test("layout remains correct when switching between PR watcher tasks with plan mode", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    // --- Seed workflow ---
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "PR Watcher Layout Workflow",
    );

    const reviewStep = await apiClient.createWorkflowStep(workflow.id, "Review", 0);

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      enable_preview_on_click: false,
    });

    // --- Seed mock GitHub data ---
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");

    // Add 3 PRs with requested_reviewers so the review watcher picks them up
    await apiClient.mockGitHubAddPRs([
      {
        number: 101,
        title: "Fix auth bug",
        state: "open",
        head_branch: "fix/auth",
        base_branch: "main",
        author_login: "alice",
        repo_owner: "testorg",
        repo_name: "testrepo",
        additions: 10,
        deletions: 2,
        requested_reviewers: [{ login: "test-user", type: "User" }],
      },
      {
        number: 202,
        title: "Add dashboard",
        state: "open",
        head_branch: "feat/dashboard",
        base_branch: "main",
        author_login: "bob",
        repo_owner: "testorg",
        repo_name: "testrepo",
        additions: 80,
        deletions: 5,
        requested_reviewers: [{ login: "test-user", type: "User" }],
      },
      {
        number: 303,
        title: "Update docs",
        state: "open",
        head_branch: "docs/update",
        base_branch: "main",
        author_login: "charlie",
        repo_owner: "testorg",
        repo_name: "testrepo",
        additions: 30,
        deletions: 10,
        requested_reviewers: [{ login: "test-user", type: "User" }],
      },
    ]);

    // Navigate to kanban BEFORE creating the review watch so the WS is
    // subscribed when task.created/task.updated events fire.
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // --- Create review watch (triggers initial poll → auto-creates 3 tasks) ---
    await apiClient.createReviewWatch(
      seedData.workspaceId,
      workflow.id,
      reviewStep.id,
      seedData.agentProfileId,
      {
        repos: [{ owner: "testorg", name: "testrepo" }],
        prompt: "Review {{pr.link}}",
      },
    );

    // --- Wait for all 3 PR tasks to appear in the Review column ---
    const prTask1Title = "PR #101: Fix auth bug";
    const prTask2Title = "PR #202: Add dashboard";
    const prTask3Title = "PR #303: Update docs";

    await expect(kanban.taskCardInColumn(prTask1Title, reviewStep.id)).toBeVisible({
      timeout: 60_000,
    });
    await expect(kanban.taskCardInColumn(prTask2Title, reviewStep.id)).toBeVisible({
      timeout: 60_000,
    });
    await expect(kanban.taskCardInColumn(prTask3Title, reviewStep.id)).toBeVisible({
      timeout: 60_000,
    });

    // --- Click PR task 1 to enter session view ---
    await kanban.taskCardInColumn(prTask1Title, reviewStep.id).click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Verify initial default layout for task 1
    await expect(session.chat).toBeVisible({ timeout: 10_000 });
    await expect(session.sidebar).toBeVisible();

    // --- Toggle plan mode on task 1 ---
    await session.togglePlanMode();
    await expect(session.planPanel).toBeVisible({ timeout: 10_000 });
    await expect(session.chat).toBeVisible();
    await expect(session.sidebar).toBeVisible();

    // --- Switch to PR task 2 via sidebar ---
    await session.taskInSidebar(prTask2Title).click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 30_000 });

    // Task 2: full layout intact — sidebar, chat accessible, changes panel visible,
    // plan panel must NOT leak from task 1, and layout fills the viewport
    await expect(session.sidebar).toBeVisible({ timeout: 15_000 });
    await session.expectNoLayoutGap();
    await expect(session.planPanel).not.toBeVisible({ timeout: 5_000 });
    // Changes panel verifies the right column rendered
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });
    // Chat may be behind a PR detail tab — click Agent tab to verify it exists
    await session.clickTab("Agent");
    await expect(session.chat).toBeVisible({ timeout: 10_000 });

    // --- Switch to PR task 3 via sidebar ---
    await session.taskInSidebar(prTask3Title).click({ timeout: 15_000 });
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 30_000 });

    // Task 3: same layout checks
    await expect(session.sidebar).toBeVisible({ timeout: 15_000 });
    await session.expectNoLayoutGap();
    await expect(session.planPanel).not.toBeVisible({ timeout: 5_000 });
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });
    await session.clickTab("Agent");
    await expect(session.chat).toBeVisible({ timeout: 10_000 });
  });
});
