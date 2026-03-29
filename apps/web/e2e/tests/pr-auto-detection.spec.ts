import path from "node:path";
import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";

/**
 * Tests the full PR auto-detection pipeline: the backend detects a PR on
 * GitHub (mock) and associates it with the task — without manually calling
 * mockGitHubAssociateTaskPR.
 *
 * Detection flow exercised:
 *   useTaskPRDetection (frontend, every 30s)
 *     → github.check_session_pr (WS action)
 *       → CheckSessionPR (backend)
 *         → resolveTaskRepo → resolvePRWatchBranch → FindPRByBranch (mock)
 *           → AssociatePRWithTask → github.task_pr.updated (WS event)
 *             → PR button appears in topbar
 *
 * Prerequisites for detection to work:
 *   - Repository has provider=github with owner/name set
 *   - TaskRepository has checkout_branch set
 *   - Mock GitHub has a PR matching the checkout branch
 */
test.describe("PR auto-detection pipeline", () => {
  test("detects PR automatically via check_session_pr", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(180_000);

    // --- Seed workflow: Inbox → Working (auto_start → Done) → Done ---
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
    // The detection pipeline needs resolveTaskRepo to return a valid owner/repo,
    // which requires provider=github with provider_owner and provider_name set.
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
    // Using `repositories` (not `repository_ids`) to set checkout_branch,
    // which resolvePRWatchBranch uses to determine which branch to search.
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
    // when task.updated events fire.
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // Move task to Working → auto_start → mock agent completes → Done
    await apiClient.moveTask(task.id, workflow.id, workingStep.id);

    await expect(kanban.taskCardInColumn("Auto-Detect PR Task", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    // --- Add PR to mock GitHub AFTER task completion ---
    // Simulates the agent having created a PR via `gh pr create`.
    // The PR's head_branch must match the task's checkout_branch ("main").
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
    // This mounts the ChangesPanelContent which runs useTaskPRDetection.
    // The hook calls github.check_session_pr immediately and every 30s.
    // The backend's CheckSessionPR resolves the repo (testorg/testrepo)
    // and branch ("main"), searches mock GitHub, and associates the PR.
    await kanban.taskCardInColumn("Auto-Detect PR Task", doneStep.id).click();
    await expect(testPage).toHaveURL(/\/[st]\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // --- Verify PR button appears in topbar ---
    // The detection hook fires immediately on mount, then every 30s.
    // Allow up to 60s for the check to succeed (accounts for git.branch
    // availability delay and the 30s polling interval).
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

    // The detection hook fires immediately on mount. Give it enough time
    // for the check to complete (a few seconds), then verify no PR button.
    // We don't need to wait a full 30s polling cycle — the immediate check
    // on mount is sufficient to confirm no PR is found.
    await expect(session.prTopbarButton()).not.toBeVisible({ timeout: 10_000 });
  });
});
