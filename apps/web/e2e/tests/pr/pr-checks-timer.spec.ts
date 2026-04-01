import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

test.describe("PR checks running timer", () => {
  /**
   * Verifies that the elapsed time for in-progress CI checks increments
   * live (not frozen) in the PR detail panel.
   *
   * Setup:
   *   Inbox -> Working (auto_start, on_turn_complete -> Done) -> Done
   *   Task with PR #201, check run "CI" in_progress with started_at in the past
   */
  test("in-progress check duration increments over time", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    // --- Seed workflow ---
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Checks Timer Workflow");
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
        number: 201,
        title: "Timer test PR",
        state: "open",
        head_branch: "feat/timer",
        head_sha: "abc123",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
      },
    ]);

    // Add an in-progress check with started_at 2 minutes ago
    const twoMinAgo = new Date(Date.now() - 2 * 60 * 1000).toISOString();
    await apiClient.mockGitHubAddCheckRuns("testorg", "testrepo", "abc123", [
      {
        name: "CI",
        status: "in_progress",
        started_at: twoMinAgo,
      },
    ]);

    // --- Create task ---
    const task = await apiClient.createTask(seedData.workspaceId, "Timer Check Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // Move task to Working -> auto_start -> Done
    await apiClient.moveTask(task.id, workflow.id, workingStep.id);

    // Associate PR with task
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 201,
      pr_url: "https://github.com/testorg/testrepo/pull/201",
      pr_title: "Timer test PR",
      head_branch: "feat/timer",
      base_branch: "main",
      author_login: "test-user",
    });

    // Wait for task to reach Done
    await expect(kanban.taskCardInColumn("Timer Check Task", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    // --- Open task ---
    await kanban.taskCardInColumn("Timer Check Task", doneStep.id).click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.idleInput().waitFor({ state: "visible", timeout: 30_000 });

    // Open PR detail panel
    await expect(session.prTopbarButton()).toBeVisible({ timeout: 15_000 });
    await session.prTopbarButton().click();
    await expect(session.prDetailPanel()).toBeVisible({ timeout: 10_000 });

    // Find the running check's duration element
    const durationEl = testPage.getByTestId("check-duration-CI");
    await expect(durationEl).toBeVisible({ timeout: 10_000 });

    // Capture the initial text and verify it contains "running"
    const initialText = await durationEl.textContent();
    expect(initialText).toContain("running");

    // Wait ~3 seconds and verify the time has changed (incremented)
    await testPage.waitForTimeout(3_000);
    const updatedText = await durationEl.textContent();
    expect(updatedText).toContain("running");
    expect(updatedText).not.toBe(initialText);
  });
});
