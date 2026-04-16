import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

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
    // createReviewWatch already kicked off an initial poll; this explicit
    // trigger is belt-and-suspenders. We do NOT assert on new_prs here
    // because the atomic dedup reservation means whichever poll runs first
    // claims the PR, and the subsequent trigger correctly returns 0 "new".
    // The real check is the task-exists poll below.
    await apiClient.triggerReviewWatch(watch.id);

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

  /**
   * When the user already opened a PR task and the agent completed,
   * triggering the watch after the PR is merged should NOT delete the task
   * — the user's work history is preserved, and the PR merged banner shows.
   */
  test("preserves started task when PR is merged (shows banner instead)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    // --- Setup mock GitHub ---
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAddPRs([
      {
        number: 202,
        title: "Reviewed feature",
        state: "open",
        head_branch: "feature/reviewed",
        base_branch: "main",
        author_login: "contributor",
        repo_owner: "testorg",
        repo_name: "testrepo",
        requested_reviewers: [{ login: "test-user", type: "User" }],
      },
    ]);

    // Create workflow: Inbox → Working (auto-start + move to Done) → Done
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Review Workflow");
    const inboxStep = await apiClient.createWorkflowStep(workflow.id, "Inbox", 0);
    const workingStep = await apiClient.createWorkflowStep(workflow.id, "Working", 1);
    const doneStep = await apiClient.createWorkflowStep(workflow.id, "Done", 2);
    await apiClient.updateWorkflowStep(workingStep.id, {
      prompt: 'e2e:message("review complete")\n{{task_prompt}}',
      events: {
        on_enter: [{ type: "auto_start_agent" }],
        on_turn_complete: [{ type: "move_to_step", config: { step_id: doneStep.id } }],
      },
    });
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
    });

    // Create review watch on the inbox step
    const watch = await apiClient.createReviewWatch(
      seedData.workspaceId,
      workflow.id,
      inboxStep.id,
      seedData.agentProfileId,
      { repos: [{ owner: "testorg", name: "testrepo" }] },
    );

    // Trigger → task created in Inbox
    await apiClient.triggerReviewWatch(watch.id);

    let prTask: { id: string; title: string } | undefined;
    await expect
      .poll(
        async () => {
          const { tasks } = await apiClient.listTasks(seedData.workspaceId);
          prTask = tasks.find((t) => t.title.includes("PR #202"));
          return prTask;
        },
        { timeout: 15_000 },
      )
      .toBeTruthy();

    // Navigate to kanban
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCardInColumn("PR #202: Reviewed feature", inboxStep.id)).toBeVisible({
      timeout: 15_000,
    });

    // Move task to Working → auto-start → completes → Done (task now has sessions)
    await apiClient.moveTask(prTask!.id, workflow.id, workingStep.id);
    await expect(kanban.taskCardInColumn("PR #202: Reviewed feature", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    // Simulate PR merged
    await apiClient.mockGitHubAddPRs([
      {
        number: 202,
        title: "Reviewed feature",
        state: "closed",
        head_branch: "feature/reviewed",
        base_branch: "main",
        author_login: "contributor",
        repo_owner: "testorg",
        repo_name: "testrepo",
      },
    ]);

    // Trigger watch → should NOT delete task (user already worked on it)
    await apiClient.triggerReviewWatch(watch.id);

    // Task should still be visible
    await expect(kanban.taskCardByTitle("PR #202: Reviewed feature")).toBeVisible({
      timeout: 5_000,
    });

    // Confirm via API
    const { tasks: tasksAfter } = await apiClient.listTasks(seedData.workspaceId);
    expect(tasksAfter.find((t) => t.title.includes("PR #202"))).toBeTruthy();
  });

  /**
   * When the user goes to GitHub directly and approves the PR (without using
   * the Kandev task), the task should be auto-deleted since the review is done.
   */
  test("auto-deletes task when PR is approved on GitHub", async ({
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
        number: 303,
        title: "Quick fix to approve",
        state: "open",
        head_branch: "fix/quick",
        base_branch: "main",
        author_login: "contributor",
        repo_owner: "testorg",
        repo_name: "testrepo",
        requested_reviewers: [{ login: "test-user", type: "User" }],
      },
    ]);

    // Create a step without auto-start
    const inboxStep = await apiClient.createWorkflowStep(seedData.workflowId, "Review Inbox", 0);

    const watch = await apiClient.createReviewWatch(
      seedData.workspaceId,
      seedData.workflowId,
      inboxStep.id,
      seedData.agentProfileId,
      { repos: [{ owner: "testorg", name: "testrepo" }] },
    );

    // Trigger → task created
    await apiClient.triggerReviewWatch(watch.id);

    await expect
      .poll(
        async () => {
          const { tasks } = await apiClient.listTasks(seedData.workspaceId);
          return tasks.find((t) => t.title.includes("PR #303"));
        },
        { timeout: 15_000 },
      )
      .toBeTruthy();

    // Navigate to kanban and verify task visible
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCardByTitle("PR #303: Quick fix to approve")).toBeVisible({
      timeout: 15_000,
    });

    // --- Simulate: user approved the PR on GitHub directly ---
    await apiClient.mockGitHubAddReviews("testorg", "testrepo", 303, [
      {
        id: 1,
        author: "test-user",
        state: "APPROVED",
        body: "LGTM",
        created_at: new Date().toISOString(),
      },
    ]);

    // Trigger watch → should detect approved PR and delete the task
    await apiClient.triggerReviewWatch(watch.id);

    // Verify task was deleted
    await expect(kanban.taskCardByTitle("PR #303: Quick fix to approve")).not.toBeVisible({
      timeout: 15_000,
    });

    const { tasks: tasksAfterCleanup } = await apiClient.listTasks(seedData.workspaceId);
    expect(tasksAfterCleanup.find((t) => t.title.includes("PR #303"))).toBeUndefined();
  });
});
