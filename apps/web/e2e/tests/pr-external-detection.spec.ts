import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";

test.describe("PR external detection", () => {
  /**
   * Verifies that when a PR is created externally (e.g. via `gh pr create`)
   * after a task session is already running, the PR button appears in the
   * topbar and the Changes panel shows the PR files and commits.
   *
   * The frontend re-fetches PR data when switching between tasks, so we
   * trigger detection by switching to another task and back rather than
   * waiting for the 30-second polling interval.
   *
   * Setup:
   *   Inbox → Working (auto_start, on_turn_complete → Done) → Done
   *   2 tasks: Feature Task (gets PR mid-session), Helper Task (switch target)
   */
  test("shows PR button and changes after external PR creation", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    // --- Seed workflow ---
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "PR Detection Workflow");

    const inboxStep = await apiClient.createWorkflowStep(workflow.id, "Inbox", 0);
    const workingStep = await apiClient.createWorkflowStep(workflow.id, "Working", 1);
    const doneStep = await apiClient.createWorkflowStep(workflow.id, "Done", 2);

    await apiClient.updateWorkflowStep(workingStep.id, {
      prompt: 'e2e:message("done")\n{{task_prompt}}',
      events: {
        on_enter: [{ type: "auto_start_agent" }],
        on_turn_complete: [{ type: "move_to_step", config: { step_id: doneStep.id } }],
      },
    });

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      enable_preview_on_click: false,
    });

    // --- Setup mock GitHub ---
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");

    // --- Create 2 tasks ---
    const featureTask = await apiClient.createTask(seedData.workspaceId, "Feature Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });
    const helperTask = await apiClient.createTask(seedData.workspaceId, "Helper Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });

    // Navigate to kanban BEFORE moving tasks so the WebSocket is already
    // subscribed when task.updated events fire. The mock agent completes in
    // <100ms; if we navigate after moveTask the events are emitted before
    // the browser subscribes and the kanban never receives them.
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // Move tasks to Working → auto_start → mock agent completes → Done
    await apiClient.moveTask(featureTask.id, workflow.id, workingStep.id);
    await apiClient.moveTask(helperTask.id, workflow.id, workingStep.id);

    await expect(kanban.taskCardInColumn("Feature Task", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });
    await expect(kanban.taskCardInColumn("Helper Task", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    // --- Open Feature Task session ---
    await kanban.taskCardInColumn("Feature Task", doneStep.id).click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // --- Verify NO PR button initially ---
    await expect(session.prTopbarButton()).not.toBeVisible({ timeout: 5_000 });

    // --- Simulate external PR creation ---
    // (e.g. developer runs `gh pr create` from the session's branch)
    await apiClient.mockGitHubAddPRs([
      {
        number: 42,
        title: "Add feature X",
        state: "open",
        head_branch: "feat/feature-x",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
        additions: 50,
        deletions: 10,
      },
    ]);

    await apiClient.mockGitHubAddPRFiles("testorg", "testrepo", 42, [
      { filename: "feature.ts", status: "added", additions: 40, deletions: 0 },
      { filename: "index.ts", status: "modified", additions: 10, deletions: 10 },
    ]);
    await apiClient.mockGitHubAddPRCommits("testorg", "testrepo", 42, [
      {
        sha: "abc1111222233334444555566667777aaaabbbb",
        message: "implement feature X",
        author_login: "test-user",
        author_date: "2026-03-03T12:00:00Z",
      },
    ]);

    // Associate PR with task (simulates what the backend poller/watcher would do
    // when it detects a PR on the session's branch)
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: featureTask.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 42,
      pr_url: "https://github.com/testorg/testrepo/pull/42",
      pr_title: "Add feature X",
      head_branch: "feat/feature-x",
      base_branch: "main",
      author_login: "test-user",
      additions: 50,
      deletions: 10,
    });

    // --- Switch to Helper Task and back to trigger PR re-fetch ---
    // The useTaskPR hook fetches on mount when no PR is in the store.
    // Switching tasks changes activeTaskId, so switching back triggers a fresh fetch.
    await session.taskInSidebar("Helper Task").click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    await session.taskInSidebar("Feature Task").click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    // --- Verify PR button appears in topbar ---
    await expect(session.prTopbarButton()).toBeVisible({ timeout: 15_000 });
    await expect(session.prTopbarButton()).toContainText("#42");

    // --- Verify PR data in Changes panel ---
    await session.clickTab("Changes");

    await expect(session.prFilesSection()).toBeVisible({ timeout: 15_000 });
    await expect(session.prFilesSection().getByText("feature.ts")).toBeVisible();
    await expect(session.prFilesSection().getByText("index.ts")).toBeVisible();

    await expect(session.prCommitsSection()).toBeVisible();
    await expect(session.prCommitsSection().getByText("implement feature X")).toBeVisible();
  });
});
