import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";

test.describe("PR watcher merged cleanup", () => {
  /**
   * When a PR watcher creates a task for a PR, and the PR is subsequently
   * merged (branch deleted), triggering the watch again should auto-delete
   * the task if the user hasn't opened it yet (no sessions).
   *
   * Setup:
   *   - Create review watch on a workflow step without auto-start
   *   - Mock a PR (open), trigger watch → task created
   *   - Change PR to merged, trigger watch again → task auto-deleted
   */
  test("auto-deletes unstarted task when PR is merged", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    // --- Setup mock GitHub ---
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAddPRs([
      {
        number: 101,
        title: "Feature to review",
        state: "open",
        head_branch: "feature/review-me",
        base_branch: "main",
        author_login: "contributor",
        repo_owner: "testorg",
        repo_name: "testrepo",
        requested_reviewers: [{ login: "test-user", type: "User" }],
      },
    ]);

    // Create a workflow step without auto-start for the review watch.
    // The seed start step may have auto_start_agent, which would create a session
    // and prevent cleanup (we only delete unstarted tasks).
    const inboxStep = await apiClient.createWorkflowStep(seedData.workflowId, "PR Inbox", 0);

    // --- Create review watch on the inbox step (no auto-start) ---
    const watch = await apiClient.createReviewWatch(
      seedData.workspaceId,
      seedData.workflowId,
      inboxStep.id,
      seedData.agentProfileId,
      { repos: [{ owner: "testorg", name: "testrepo" }] },
    );

    // --- Trigger watch → should create a task for PR #101 ---
    const triggerResult = await apiClient.triggerReviewWatch(watch.id);
    expect(triggerResult.new_prs).toBeGreaterThanOrEqual(1);

    // Task creation is async (goroutine), poll until it appears
    let prTask: { id: string; title: string } | undefined;
    await expect
      .poll(
        async () => {
          const { tasks } = await apiClient.listTasks(seedData.workspaceId);
          prTask = tasks.find((t) => t.title.includes("PR #101"));
          return prTask;
        },
        { timeout: 15_000 },
      )
      .toBeTruthy();

    // Navigate to kanban and verify task is visible
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCardByTitle("PR #101: Feature to review")).toBeVisible({
      timeout: 15_000,
    });

    // --- Simulate PR merged: update mock PR state to closed ---
    await apiClient.mockGitHubAddPRs([
      {
        number: 101,
        title: "Feature to review",
        state: "closed",
        head_branch: "feature/review-me",
        base_branch: "main",
        author_login: "contributor",
        repo_owner: "testorg",
        repo_name: "testrepo",
      },
    ]);

    // --- Trigger watch again → should detect merged PR and delete the task ---
    await apiClient.triggerReviewWatch(watch.id);

    // Verify task was deleted
    await expect(kanban.taskCardByTitle("PR #101: Feature to review")).not.toBeVisible({
      timeout: 15_000,
    });

    // Confirm via API
    const { tasks: tasksAfterCleanup } = await apiClient.listTasks(seedData.workspaceId);
    const deletedTask = tasksAfterCleanup.find((t) => t.title.includes("PR #101"));
    expect(deletedTask).toBeUndefined();
  });
});
