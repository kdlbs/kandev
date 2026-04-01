import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

test.describe("PR detail panel auto-show", () => {
  /**
   * Verifies that the PR detail panel automatically appears as a tab in
   * the center group when a task has an associated pull request.
   *
   * Setup:
   *   Inbox → Working (auto_start, on_turn_complete → Done) → Done
   *   Task A (with PR #101)
   */
  test("auto-shows PR detail tab for task with associated PR", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    // --- Seed workflow ---
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "PR Auto-Show Workflow");

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
        additions: 10,
        deletions: 2,
      },
    ]);

    // --- Create task ---
    const taskA = await apiClient.createTask(seedData.workspaceId, "Auth Fix Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });

    // Navigate to kanban BEFORE moving task
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // Move task to Working → auto_start → mock agent completes → Done
    await apiClient.moveTask(taskA.id, workflow.id, workingStep.id);

    // Associate PR with Task A only
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
      additions: 10,
      deletions: 2,
    });

    // Wait for task to reach Done
    await expect(kanban.taskCardInColumn("Auth Fix Task", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    // --- Open Task A (has PR) ---
    await kanban.taskCardInColumn("Auth Fix Task", doneStep.id).click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Wait for mock agent to complete so layout is fully settled
    await session.idleInput().waitFor({ state: "visible", timeout: 30_000 });

    // Wait for PR data to sync and panel to auto-appear
    await expect(session.prTopbarButton()).toBeVisible({ timeout: 15_000 });
    await expect(session.prDetailTab()).toBeVisible({ timeout: 10_000 });

    // Click the PR detail tab to verify it shows content
    await session.prDetailTab().click();
    await expect(session.prDetailPanel()).toBeVisible({ timeout: 10_000 });
  });
});
