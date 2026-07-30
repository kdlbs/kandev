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
    await session.waitForChatIdle({ timeout: 30_000 });

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
    await session.waitForChatIdle({ timeout: 30_000 });

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
    await session.waitForChatIdle({ timeout: 30_000 });

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
    await session.waitForChatIdle({ timeout: 30_000 });
    await expect(session.prDetailTab()).toBeVisible({ timeout: 15_000 });

    // Switch to Task B (no PR) — PR panel should be removed
    await session.clickTaskInSidebar("Plain Task");
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    await expect(session.prDetailTab()).not.toBeVisible({ timeout: 10_000 });

    // Switch back to Task A — PR panel should re-appear
    await session.clickTaskInSidebar("API Refactor Task");
    await session.waitForLoad();
    await expect(session.prDetailTab()).toBeVisible({ timeout: 15_000 });
  });

  /**
   * Regression: the PR detail panel must auto-open as a tab inside the
   * session's dockview group, not as a split in a separate group. The bug
   * triggered intermittently when opening/switching PR-linked tasks because
   * `useAutoPRPanel` used a stale `centerGroupId` from the store and fell
   * through dockview's default placement (a new group to the right).
   *
   * Setup:
   *   Inbox → Working (auto_start, on_turn_complete → Done) → Done
   *   Task A (with PR #401), Task B (with PR #402)
   */
  test("auto-shows PR panel as a tab in the session group, not a split", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "PR Placement Workflow");

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
        number: 401,
        title: "First PR",
        state: "open",
        head_branch: "feat/first",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
        additions: 10,
        deletions: 2,
      },
      {
        number: 402,
        title: "Second PR",
        state: "open",
        head_branch: "feat/second",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
        additions: 20,
        deletions: 4,
      },
    ]);

    const taskA = await apiClient.createTask(seedData.workspaceId, "First PR Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });
    const taskB = await apiClient.createTask(seedData.workspaceId, "Second PR Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await apiClient.moveTask(taskA.id, workflow.id, workingStep.id);
    await apiClient.moveTask(taskB.id, workflow.id, workingStep.id);
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: taskA.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 401,
      pr_url: "https://github.com/testorg/testrepo/pull/401",
      pr_title: "First PR",
      head_branch: "feat/first",
      base_branch: "main",
      author_login: "test-user",
      additions: 10,
      deletions: 2,
    });
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: taskB.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 402,
      pr_url: "https://github.com/testorg/testrepo/pull/402",
      pr_title: "Second PR",
      head_branch: "feat/second",
      base_branch: "main",
      author_login: "test-user",
      additions: 20,
      deletions: 4,
    });

    await expect(kanban.taskCardInColumn("First PR Task", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });
    await expect(kanban.taskCardInColumn("Second PR Task", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    // Open Task A and wait for the PR panel to auto-show.
    await kanban.taskCardInColumn("First PR Task", doneStep.id).click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    await expect(session.prDetailTab()).toBeVisible({ timeout: 15_000 });

    // Invariant: PR panel shares the session's dockview group (is a tab, not a split).
    await session.expectPrPanelAndSessionShareGroup();

    // Switch to Task B — the race path that triggered the original bug. The
    // session changes, a new layout is resolved, and the PR auto-add hook runs
    // against the new session. Assert the invariant still holds.
    await session.clickTaskInSidebar("Second PR Task");
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    await expect(session.prDetailTab()).toBeVisible({ timeout: 15_000 });
    await session.expectPrPanelAndSessionShareGroup();

    // Switch BACK to Task A — regression: the fast-path layout switch used to
    // remove the old session tab then look for a chat/session panel to anchor
    // the new tab. With the old tab gone and the PR panel being non-chat, the
    // lookup failed and fell through to a sidebar split, placing the session
    // tab in a new group while the PR panel stayed in the old center group.
    await session.clickTaskInSidebar("First PR Task");
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    await expect(session.prDetailTab()).toBeVisible({ timeout: 15_000 });
    await session.expectPrPanelAndSessionShareGroup();
  });

  /**
   * General correctness coverage for the auto-shown PR panel across task
   * switches (each task keeps its own dockview layout in this environment,
   * so this does not reproduce the exact stale-panel-reuse window flagged
   * by Greptile/cubic-dev-ai on PR #1636 — that fix is unit-tested directly
   * against `runAutoPRPanelEffect`'s resync logic in
   * use-auto-pr-panel.test.ts). This still guards the user-visible
   * contract: the panel must show the CURRENTLY active task's PR.
   *
   * Setup:
   *   Inbox → Working (auto_start, on_turn_complete → Done) → Done
   *   Task A (with PR #401), Task B (with PR #402)
   */
  test("shows the correct task's PR after switching between two PR-linked tasks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "PR Refresh Workflow");

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
        number: 401,
        title: "First task PR",
        state: "open",
        head_branch: "feat/first",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
        additions: 5,
        deletions: 1,
      },
      {
        number: 402,
        title: "Second task PR",
        state: "open",
        head_branch: "feat/second",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
        additions: 8,
        deletions: 2,
      },
    ]);

    const taskA = await apiClient.createTask(seedData.workspaceId, "Refresh Task A", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });
    const taskB = await apiClient.createTask(seedData.workspaceId, "Refresh Task B", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await apiClient.moveTask(taskA.id, workflow.id, workingStep.id);
    await apiClient.moveTask(taskB.id, workflow.id, workingStep.id);
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: taskA.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 401,
      pr_url: "https://github.com/testorg/testrepo/pull/401",
      pr_title: "First task PR",
      head_branch: "feat/first",
      base_branch: "main",
      author_login: "test-user",
      additions: 5,
      deletions: 1,
    });
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: taskB.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 402,
      pr_url: "https://github.com/testorg/testrepo/pull/402",
      pr_title: "Second task PR",
      head_branch: "feat/second",
      base_branch: "main",
      author_login: "test-user",
      additions: 8,
      deletions: 2,
    });

    await expect(kanban.taskCardInColumn("Refresh Task A", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });
    await expect(kanban.taskCardInColumn("Refresh Task B", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    await kanban.taskCardInColumn("Refresh Task A", doneStep.id).click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    await expect(session.prDetailTab()).toHaveText("PR #401", { timeout: 15_000 });

    await session.clickTaskInSidebar("Refresh Task B");
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    await expect(session.prDetailTab()).toHaveText("PR #402", { timeout: 15_000 });

    await session.clickTaskInSidebar("Refresh Task A");
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    await expect(session.prDetailTab()).toHaveText("PR #401", { timeout: 15_000 });
  });

  /**
   * Contract coverage for switching to a different task's PR while its
   * feedback (reviews/comments/checks) is still in flight: the tab/header
   * must reflect the new task's PR immediately, and once the held fetch
   * resolves, its "Bot comments" disclosure must render collapsed (not
   * inheriting whatever the previous PR's disclosure was left in).
   *
   * This harness gives each task its own isolated worktree/env, so the
   * auto-shown "pr-detail" panel is freshly mounted per task here and this
   * does not reproduce the exact same-React-instance staleness window
   * reported in production (reused instance carrying over `usePRFeedback`
   * state and `CommentsSection`'s bot-comments toggle across tasks sharing
   * one env — same category of limitation already called out on the
   * "switching between two PR-linked tasks" test above). That reused-instance
   * regression is covered deterministically by
   * `hooks/domains/github/use-pr-feedback.test.ts` (masks stale feedback by
   * PR identity) and `components/github/pr-detail-panel.test.tsx` (proves the
   * bot-comments toggle doesn't leak across a switch on one panel instance).
   * This spec still guards the real browser flow end-to-end.
   *
   * Setup:
   *   Inbox → Working (auto_start, on_turn_complete → Done) → Done
   *   Task A (PR #601, one bot comment), Task B (PR #602, one bot comment,
   *   feedback fetch held back until explicitly released)
   */
  test("shows the new task's PR immediately and renders its bot-comments disclosure collapsed while switching with feedback in flight", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "PR Leak Workflow");

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

    const OWNER = "testorg";
    const REPO = "testrepo";
    const PR_A = 601;
    const PR_B = 602;

    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAddPRs([
      {
        number: PR_A,
        title: "Leak Task A PR",
        state: "open",
        head_branch: "feat/leak-a",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: OWNER,
        repo_name: REPO,
        additions: 3,
        deletions: 1,
      },
      {
        number: PR_B,
        title: "Leak Task B PR",
        state: "open",
        head_branch: "feat/leak-b",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: OWNER,
        repo_name: REPO,
        additions: 4,
        deletions: 2,
      },
    ]);

    const taskA = await apiClient.createTask(seedData.workspaceId, "Leak Task A", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });
    const taskB = await apiClient.createTask(seedData.workspaceId, "Leak Task B", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await apiClient.moveTask(taskA.id, workflow.id, workingStep.id);
    await apiClient.moveTask(taskB.id, workflow.id, workingStep.id);
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: taskA.id,
      owner: OWNER,
      repo: REPO,
      pr_number: PR_A,
      pr_url: `https://github.com/${OWNER}/${REPO}/pull/${PR_A}`,
      pr_title: "Leak Task A PR",
      head_branch: "feat/leak-a",
      base_branch: "main",
      author_login: "test-user",
      additions: 3,
      deletions: 1,
    });
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: taskB.id,
      owner: OWNER,
      repo: REPO,
      pr_number: PR_B,
      pr_url: `https://github.com/${OWNER}/${REPO}/pull/${PR_B}`,
      pr_title: "Leak Task B PR",
      head_branch: "feat/leak-b",
      base_branch: "main",
      author_login: "test-user",
      additions: 4,
      deletions: 2,
    });

    await apiClient.mockGitHubSeedPRFeedback({
      owner: OWNER,
      repo: REPO,
      pr_number: PR_A,
      comments: [
        {
          id: 1,
          author: "review-bot",
          author_is_bot: true,
          body: "PR A bot comment",
        },
      ],
    });
    await apiClient.mockGitHubSeedPRFeedback({
      owner: OWNER,
      repo: REPO,
      pr_number: PR_B,
      comments: [
        {
          id: 2,
          author: "review-bot",
          author_is_bot: true,
          body: "PR B bot comment",
        },
      ],
    });

    await expect(kanban.taskCardInColumn("Leak Task A", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });
    await expect(kanban.taskCardInColumn("Leak Task B", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    // Hold PR B's feedback fetch until explicitly released, so the switch
    // below lands while its comments are still loading.
    let releasePRBFeedback!: () => void;
    const prBFeedbackHeld = new Promise<void>((resolve) => {
      releasePRBFeedback = resolve;
    });
    let observePRBFeedbackRequested!: () => void;
    const prBFeedbackRequested = new Promise<void>((resolve) => {
      observePRBFeedbackRequested = resolve;
    });
    await testPage.route(
      (url) => url.pathname === `/api/v1/github/prs/${OWNER}/${REPO}/${PR_B}`,
      async (route) => {
        const response = await route.fetch();
        observePRBFeedbackRequested();
        await prBFeedbackHeld;
        await route.fulfill({ response });
      },
    );

    await kanban.taskCardInColumn("Leak Task A", doneStep.id).click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    await expect(session.prDetailTab()).toHaveText(`PR #${PR_A}`, { timeout: 15_000 });
    await session.prDetailTab().click();

    const panel = session.prDetailPanel();
    await expect(panel).toBeVisible({ timeout: 10_000 });
    await expect(panel.getByText("Bot comments (1)")).toBeVisible({ timeout: 15_000 });
    await panel.getByText("Bot comments (1)").click();
    await expect(panel.getByText("PR A bot comment")).toBeVisible();

    // Switch to Task B — its feedback request is held back by the route above.
    await session.clickTaskInSidebar("Leak Task B");
    await prBFeedbackRequested;

    // The header/tab already reflects Task B's PR (comes from the task-PR
    // association, not the held feedback fetch) …
    await expect(session.prDetailTab()).toHaveText(`PR #${PR_B}`, { timeout: 15_000 });
    await session.prDetailTab().click();
    await expect(panel).toBeVisible({ timeout: 10_000 });
    // … but PR A's comment, and its expanded bot-comments disclosure, must
    // be gone immediately — not left on screen for the several seconds the
    // feedback fetch is still in flight.
    await expect(panel.getByText("PR A bot comment")).toHaveCount(0);

    releasePRBFeedback();

    // PR B's bot comment must stay collapsed by default — the disclosure
    // must not inherit Task A's expanded state — and must never be PR A's.
    await expect(panel.getByText("Bot comments (1)")).toBeVisible({ timeout: 15_000 });
    await expect(panel.getByText("PR B bot comment")).toHaveCount(0);
    await panel.getByText("Bot comments (1)").click();
    await expect(panel.getByText("PR B bot comment")).toBeVisible();
    await expect(panel.getByText("PR A bot comment")).toHaveCount(0);
  });
});
