import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

test.describe("PR auto-detection", () => {
  /**
   * Tests the full PR auto-detection pipeline: the backend detects a PR on
   * GitHub (mock) and associates it with the task — without manually calling
   * mockGitHubAssociateTaskPR.
   *
   * Detection flow exercised:
   *   useTaskPRDetection (frontend, every 30s)
   *     -> github.check_session_pr (WS action)
   *       -> CheckSessionPR (backend)
   *         -> resolveTaskRepo -> resolvePRWatchBranch -> FindPRByBranch (mock)
   *           -> AssociatePRWithTask -> github.task_pr.updated (WS event)
   *             -> PR button appears in topbar
   *
   * Prerequisites for detection to work:
   *   - Repository has provider=github with owner/name set
   *   - TaskRepository has checkout_branch set
   *   - Mock GitHub has a PR matching the checkout branch
   */
  test("detects PR automatically via check_session_pr", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(180_000);

    // --- Seed workflow: Inbox -> Working (auto_start -> Done) -> Done ---
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "PR Auto-Detection Workflow",
    );

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

    // --- Create repository with GitHub provider info ---
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const githubRepo = await apiClient.createRepository(seedData.workspaceId, repoDir, "main", {
      name: "GitHub Test Repo",
      provider: "github",
      provider_owner: "testorg",
      provider_name: "testrepo",
    });

    // --- Setup mock GitHub ---
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");

    // --- Create task with checkout_branch set ---
    const task = await apiClient.createTask(seedData.workspaceId, "Auto-Detect PR Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: seedData.agentProfileId,
      repositories: [
        {
          repository_id: githubRepo.id,
          checkout_branch: "main",
        },
      ],
    });

    // Navigate to kanban BEFORE moving tasks so the WebSocket is subscribed
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // Move task to Working -> auto_start -> mock agent completes -> Done
    await apiClient.moveTask(task.id, workflow.id, workingStep.id);

    await expect(kanban.taskCardInColumn("Auto-Detect PR Task", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    // --- Add PR to mock GitHub AFTER task completion ---
    await apiClient.mockGitHubAddPRs([
      {
        number: 99,
        title: "Agent-created PR",
        state: "open",
        head_branch: "main",
        base_branch: "develop",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
        additions: 30,
        deletions: 5,
      },
    ]);

    // --- Open the task session ---
    await kanban.taskCardInColumn("Auto-Detect PR Task", doneStep.id).click();
    await expect(testPage).toHaveURL(/\/[st]\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // --- Verify PR button appears in topbar ---
    await expect(session.prTopbarButton()).toBeVisible({ timeout: 60_000 });
    await expect(session.prTopbarButton()).toContainText("#99");
  });

  test("does not show PR for tasks without a matching PR", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    // --- Seed workflow ---
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "PR Negative Workflow");

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

    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const githubRepo = await apiClient.createRepository(seedData.workspaceId, repoDir, "main", {
      name: "GitHub No-PR Repo",
      provider: "github",
      provider_owner: "testorg",
      provider_name: "testrepo",
    });

    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    // No PRs added to mock GitHub — detection should find nothing

    const task = await apiClient.createTask(seedData.workspaceId, "No PR Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: seedData.agentProfileId,
      repositories: [
        {
          repository_id: githubRepo.id,
          checkout_branch: "main",
        },
      ],
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await apiClient.moveTask(task.id, workflow.id, workingStep.id);
    await expect(kanban.taskCardInColumn("No PR Task", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    // Open the session
    await kanban.taskCardInColumn("No PR Task", doneStep.id).click();
    await expect(testPage).toHaveURL(/\/[st]\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Verify no PR button
    await expect(session.prTopbarButton()).not.toBeVisible({ timeout: 10_000 });
  });
});

test.describe("PR external detection", () => {
  /**
   * Verifies that when a PR is created externally (e.g. via `gh pr create`)
   * after a task session is already running, the PR button appears in the
   * topbar and the Changes panel shows the PR files and commits.
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

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // Move tasks to Working -> auto_start -> mock agent completes -> Done
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
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // --- Verify NO PR button initially ---
    await expect(session.prTopbarButton()).not.toBeVisible({ timeout: 5_000 });

    // --- Simulate external PR creation ---
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

    // Associate PR with task
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
    await session.taskInSidebar("Helper Task").click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    await session.taskInSidebar("Feature Task").click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    // --- Verify PR button appears in topbar ---
    await expect(session.prTopbarButton()).toBeVisible({ timeout: 15_000 });
    await expect(session.prTopbarButton()).toContainText("#42");

    // --- Verify PR data in Changes panel ---
    await session.clickTab("Changes");

    await expect(session.prFilesSection()).toBeVisible({ timeout: 15_000 });
    await expect(session.prFilesSection().getByText("feature.ts")).toBeVisible();
    await expect(session.prFilesSection().getByText("index.ts")).toBeVisible();

    await expect(session.commitsSection()).toBeVisible();
    await expect(session.commitsSection().getByText("implement feature X")).toBeVisible();
  });
});
