import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

test.describe("PR detail panel", () => {
  /**
   * Verifies that the PR detail panel can be opened via the topbar button
   * when a task has an associated pull request, and renders PR content.
   *
   * Setup:
   *   Inbox → Working (auto_start, on_turn_complete → Done) → Done
   *   Task A (with PR #101)
   */
  test("opens PR detail panel via topbar button", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(120_000);

    // --- Seed workflow ---
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "PR Panel Workflow");

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

    // Associate PR with Task A
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

    // --- Open Task A ---
    await kanban.taskCardInColumn("Auth Fix Task", doneStep.id).click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.idleInput().waitFor({ state: "visible", timeout: 30_000 });

    // Verify PR topbar button appears with PR number
    await expect(session.prTopbarButton()).toBeVisible({ timeout: 15_000 });
    await expect(session.prTopbarButton()).toContainText("#101");

    // Click PR topbar button to open the PR detail panel
    await session.prTopbarButton().click();

    // Verify PR detail panel renders with content
    await expect(session.prDetailPanel()).toBeVisible({ timeout: 10_000 });
  });

  /**
   * Verifies that the PR detail panel auto-appears when a task has an
   * associated PR, and that dismissing it persists across page reload.
   *
   * Setup:
   *   Inbox → Working (auto_start, on_turn_complete → Done) → Done
   *   Task A (with PR #201)
   */
  test("auto-shows PR panel and persists dismissal across reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "PR Dismiss Workflow");

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
        number: 201,
        title: "Add feature",
        state: "open",
        head_branch: "feat/new",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
        additions: 20,
        deletions: 5,
      },
    ]);

    const taskA = await apiClient.createTask(seedData.workspaceId, "Feature Task", {
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
      pr_number: 201,
      pr_url: "https://github.com/testorg/testrepo/pull/201",
      pr_title: "Add feature",
      head_branch: "feat/new",
      base_branch: "main",
      author_login: "test-user",
      additions: 20,
      deletions: 5,
    });

    await expect(kanban.taskCardInColumn("Feature Task", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    // Open task
    await kanban.taskCardInColumn("Feature Task", doneStep.id).click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.idleInput().waitFor({ state: "visible", timeout: 30_000 });

    // Wait for PR data to arrive (topbar button confirms PR is loaded)
    await expect(session.prTopbarButton()).toBeVisible({ timeout: 15_000 });

    // 1. PR panel should auto-appear (no button click needed)
    await expect(session.prDetailTab()).toBeVisible({ timeout: 15_000 });

    // 2. Dismiss the PR panel by closing its tab (hover to reveal close button)
    await session.prDetailTab().hover();
    const closeBtn = session.prDetailTab().locator(".dv-default-tab-action");
    await closeBtn.click();
    await expect(session.prDetailTab()).not.toBeVisible({ timeout: 10_000 });

    // 3. Reload page — panel should stay dismissed
    await testPage.reload();
    await session.waitForLoad();
    await session.idleInput().waitFor({ state: "visible", timeout: 30_000 });

    // Give the auto-show hook time to fire (double rAF) — panel should NOT reappear
    await testPage.waitForTimeout(1_000);
    await expect(session.prDetailTab()).not.toBeVisible();

    // 4. Can still open it manually via topbar button
    await expect(session.prTopbarButton()).toBeVisible({ timeout: 15_000 });
    await session.prTopbarButton().click();
    await expect(session.prDetailPanel()).toBeVisible({ timeout: 10_000 });
  });

  /**
   * Verifies that the PR panel is removed when switching to a task without a PR,
   * and re-appears when switching back to a task with a PR.
   *
   * Setup:
   *   Inbox → Working (auto_start, on_turn_complete → Done) → Done
   *   Task A (with PR #301), Task B (no PR)
   */
  test("removes PR panel when switching to task without PR", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "PR Switch Workflow");

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
        number: 301,
        title: "Refactor API",
        state: "open",
        head_branch: "refactor/api",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
        additions: 30,
        deletions: 10,
      },
    ]);

    // Create two tasks — only Task A gets a PR
    const taskA = await apiClient.createTask(seedData.workspaceId, "API Refactor Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });
    const taskB = await apiClient.createTask(seedData.workspaceId, "Plain Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // Move both tasks and associate PR only with Task A
    await apiClient.moveTask(taskA.id, workflow.id, workingStep.id);
    await apiClient.moveTask(taskB.id, workflow.id, workingStep.id);
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: taskA.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 301,
      pr_url: "https://github.com/testorg/testrepo/pull/301",
      pr_title: "Refactor API",
      head_branch: "refactor/api",
      base_branch: "main",
      author_login: "test-user",
      additions: 30,
      deletions: 10,
    });

    // Wait for both tasks to reach Done
    await expect(kanban.taskCardInColumn("API Refactor Task", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });
    await expect(kanban.taskCardInColumn("Plain Task", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    // Open Task A (has PR) — PR panel should auto-appear
    await kanban.taskCardInColumn("API Refactor Task", doneStep.id).click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.idleInput().waitFor({ state: "visible", timeout: 30_000 });
    await expect(session.prDetailTab()).toBeVisible({ timeout: 15_000 });

    // Switch to Task B (no PR) — PR panel should be removed
    await session.clickTaskInSidebar("Plain Task");
    await session.waitForLoad();
    await session.idleInput().waitFor({ state: "visible", timeout: 30_000 });
    await expect(session.prDetailTab()).not.toBeVisible({ timeout: 10_000 });

    // Switch back to Task A — PR panel should re-appear
    await session.clickTaskInSidebar("API Refactor Task");
    await session.waitForLoad();
    await expect(session.prDetailTab()).toBeVisible({ timeout: 15_000 });
  });
});
