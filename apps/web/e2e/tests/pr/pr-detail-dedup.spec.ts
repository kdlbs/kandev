import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

test.describe("PR detail panel dedup", () => {
  /**
   * Regression: clicking the topbar PR button when the PR panel is already
   * auto-shown must focus the existing tab, not create a duplicate. The bug
   * was that auto-add used panel id `pr-detail` while the topbar button used
   * the keyed id `pr-detail|<owner>/<repo>/<n>`, so `focusOrAddPanel`'s id
   * dedup missed and the user ended up with two "Pull Request" tabs.
   *
   * Setup:
   *   Inbox → Working (auto_start, on_turn_complete → Done) → Done
   *   Task A (with PR #501)
   */
  test("topbar PR button focuses the auto-shown panel instead of duplicating it", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "PR Dedup Workflow");

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

    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAddPRs([
      {
        number: 501,
        title: "Dedup test PR",
        state: "open",
        head_branch: "fix/dedup",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
        additions: 5,
        deletions: 1,
      },
    ]);

    const taskA = await apiClient.createTask(seedData.workspaceId, "Dedup PR Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await apiClient.moveTask(taskA.id, workflow.id, workingStep.id);
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: taskA.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 501,
      pr_url: "https://github.com/testorg/testrepo/pull/501",
      pr_title: "Dedup test PR",
      head_branch: "fix/dedup",
      base_branch: "main",
      author_login: "test-user",
      additions: 5,
      deletions: 1,
    });

    await expect(kanban.taskCardInColumn("Dedup PR Task", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    // Open task — auto-show should produce the PR tab.
    await kanban.taskCardInColumn("Dedup PR Task", doneStep.id).click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.idleInput().waitFor({ state: "visible", timeout: 30_000 });

    // PR auto-panel is open before any click on the topbar button.
    await expect(session.prDetailTab()).toBeVisible({ timeout: 15_000 });
    await expect(session.prDetailTab()).toHaveCount(1);
    await expect(session.prTopbarButton()).toBeVisible({ timeout: 15_000 });

    // Click the topbar PR button — must focus the existing tab, not create a second one.
    await session.prTopbarButton().click();

    // Regression assertion: still exactly one PR tab. `toHaveCount` polls, so
    // a regression that adds a second tab would be caught here without a sleep.
    await expect(session.prDetailTab()).toHaveCount(1);
    await expect(session.prDetailPanel()).toBeVisible();
  });
});
