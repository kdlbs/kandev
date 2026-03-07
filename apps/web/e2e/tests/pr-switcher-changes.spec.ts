import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";

test.describe("PR switcher changes panel", () => {
  /**
   * Verifies that the changes panel shows the correct PR files and commits
   * for each task when switching between tasks in the sidebar.
   *
   * Also verifies that a task without a PR association does NOT show the
   * PR Changes or PR Commits sections.
   *
   * Setup:
   *   Inbox → Working (auto_start, on_turn_complete → Done) → Done
   *   3 tasks: Task A (PR #101), Task B (PR #202), Task C (no PR)
   */
  test("shows correct PR data per task and hides PR sections for tasks without a PR", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    // --- Seed workflow ---
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "PR Switcher Workflow");

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

    // --- Seed mock GitHub data ---
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");

    // PR #101 files and commits
    await apiClient.mockGitHubAddPRs([
      {
        number: 101,
        title: "Fix auth bug",
        state: "open",
        head_branch: "fix/auth",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
        additions: 35,
        deletions: 3,
      },
      {
        number: 202,
        title: "Add dashboard",
        state: "open",
        head_branch: "feat/dashboard",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
        additions: 125,
        deletions: 5,
      },
    ]);

    await apiClient.mockGitHubAddPRFiles("testorg", "testrepo", 101, [
      { filename: "auth.go", status: "modified", additions: 10, deletions: 3 },
      { filename: "auth_test.go", status: "added", additions: 25, deletions: 0 },
    ]);
    await apiClient.mockGitHubAddPRCommits("testorg", "testrepo", 101, [
      {
        sha: "aaa1111222233334444555566667777aaaabbbb",
        message: "fix auth token expiry",
        author_login: "test-user",
        author_date: "2026-03-01T12:00:00Z",
      },
    ]);

    await apiClient.mockGitHubAddPRFiles("testorg", "testrepo", 202, [
      { filename: "dashboard.tsx", status: "added", additions: 80, deletions: 0 },
      { filename: "api.ts", status: "modified", additions: 15, deletions: 5 },
      { filename: "styles.css", status: "added", additions: 30, deletions: 0 },
    ]);
    await apiClient.mockGitHubAddPRCommits("testorg", "testrepo", 202, [
      {
        sha: "bbb1111222233334444555566667777aaaabbbb",
        message: "add dashboard component",
        author_login: "test-user",
        author_date: "2026-03-01T10:00:00Z",
      },
      {
        sha: "ccc1111222233334444555566667777aaaabbbb",
        message: "add api client",
        author_login: "test-user",
        author_date: "2026-03-01T11:00:00Z",
      },
    ]);

    // --- Create 3 tasks ---
    const taskA = await apiClient.createTask(seedData.workspaceId, "Auth Fix Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });
    const taskB = await apiClient.createTask(seedData.workspaceId, "Dashboard Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });
    const taskC = await apiClient.createTask(seedData.workspaceId, "No PR Task", {
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

    // Move all tasks to Working → auto_start → mock agent completes → Done
    await apiClient.moveTask(taskA.id, workflow.id, workingStep.id);
    await apiClient.moveTask(taskB.id, workflow.id, workingStep.id);
    await apiClient.moveTask(taskC.id, workflow.id, workingStep.id);

    // Associate PRs with tasks (Task C has no PR)
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: taskA.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 101,
      pr_url: "https://github.com/testorg/testrepo/pull/101",
      pr_title: "Fix auth bug",
      head_branch: "fix/auth",
      base_branch: "main",
      author_login: "test-user",
      additions: 35,
      deletions: 3,
    });
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: taskB.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 202,
      pr_url: "https://github.com/testorg/testrepo/pull/202",
      pr_title: "Add dashboard",
      head_branch: "feat/dashboard",
      base_branch: "main",
      author_login: "test-user",
      additions: 125,
      deletions: 5,
    });

    // Wait for all three tasks to reach Done
    await expect(kanban.taskCardInColumn("Auth Fix Task", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });
    await expect(kanban.taskCardInColumn("Dashboard Task", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });
    await expect(kanban.taskCardInColumn("No PR Task", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    // --- Click Task A to enter session view ---
    await kanban.taskCardInColumn("Auth Fix Task", doneStep.id).click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // --- Switch to the Changes tab (Files tab is active by default) ---
    await session.clickTab("Changes");

    // --- Verify Task A PR data ---
    await expect(session.prFilesSection()).toBeVisible({ timeout: 15_000 });
    await expect(session.prFilesSection().getByText("auth.go")).toBeVisible();
    await expect(session.prFilesSection().getByText("auth_test.go")).toBeVisible();

    await expect(session.prCommitsSection()).toBeVisible();
    await expect(session.prCommitsSection().getByText("fix auth token expiry")).toBeVisible();

    // --- Switch to Task B ---
    await session.taskInSidebar("Dashboard Task").click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });
    await session.clickTab("Changes");

    // Wait for PR data to load for Task B
    await expect(session.prFilesSection()).toBeVisible({ timeout: 15_000 });
    await expect(session.prFilesSection().getByText("dashboard.tsx")).toBeVisible();
    await expect(session.prFilesSection().getByText("api.ts")).toBeVisible();
    await expect(session.prFilesSection().getByText("styles.css")).toBeVisible();

    // Verify Task A files are NOT visible
    await expect(session.prFilesSection().getByText("auth.go")).not.toBeVisible();

    await expect(session.prCommitsSection()).toBeVisible();
    await expect(session.prCommitsSection().getByText("add dashboard component")).toBeVisible();
    await expect(session.prCommitsSection().getByText("add api client")).toBeVisible();

    // --- Switch to Task C (no PR) ---
    await session.taskInSidebar("No PR Task").click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });
    await session.clickTab("Changes");

    // PR sections should NOT be visible for a task without a PR
    await expect(session.prFilesSection()).not.toBeVisible({ timeout: 10_000 });
    await expect(session.prCommitsSection()).not.toBeVisible();

    // --- Switch back to Task A to confirm data reappears ---
    await session.taskInSidebar("Auth Fix Task").click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });
    await session.clickTab("Changes");

    await expect(session.prFilesSection()).toBeVisible({ timeout: 15_000 });
    await expect(session.prFilesSection().getByText("auth.go")).toBeVisible();
    await expect(session.prFilesSection().getByText("auth_test.go")).toBeVisible();
    await expect(session.prCommitsSection().getByText("fix auth token expiry")).toBeVisible();
  });
});
