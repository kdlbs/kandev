import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

test.describe("PR watcher auto-start on move", () => {
  /**
   * Race condition: When a task created from a PR watcher sits on a step
   * WITHOUT auto_start_agent, the user opens the task (creating a CREATED
   * session with workspace-only preparation), then moves the task to a step
   * WITH auto_start_agent on the kanban board.
   *
   * BUG: The on_enter auto_start_agent action calls PromptTask which rejects
   * CREATED sessions, causing the prompt to be queued instead of starting
   * the agent. The queued prompt is never consumed because no agent is running.
   *
   * EXPECTED: The agent starts immediately on the prepared workspace.
   *
   * Flow:
   *   1. Create workflow: Waiting (no auto_start) → Review (auto_start) → Done
   *   2. Create task as PR watcher would (metadata.agent_profile_id)
   *   3. Open task → session auto-prepares (CREATED, workspace-only)
   *   4. Go back to kanban
   *   5. Move task from Waiting to Review via API
   *   6. Open the task
   *   7. Assert: agent starts and completes (NOT queued)
   */
  test("starts agent immediately when moving task with CREATED session to auto-start step", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    // --- Create workflow: Waiting → Review (auto_start) → Done ---
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "PR Move Workflow");

    const waitingStep = await apiClient.createWorkflowStep(workflow.id, "Waiting", 0);
    const reviewStep = await apiClient.createWorkflowStep(workflow.id, "Review", 1);
    const doneStep = await apiClient.createWorkflowStep(workflow.id, "Done", 2);

    // Configure Review step: auto_start + move to Done on turn complete
    await apiClient.updateWorkflowStep(reviewStep.id, {
      prompt: '/e2e:message("review complete")\n{{task_prompt}}',
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

    // --- Create task as PR watcher would ---
    const task = await apiClient.createTask(seedData.workspaceId, "PR #55: Refactor auth", {
      description: "Review the auth refactoring changes",
      workflow_id: workflow.id,
      workflow_step_id: waitingStep.id,
      repositories: [
        {
          repository_id: seedData.repositoryId,
          base_branch: "main",
        },
      ],
      metadata: {
        agent_profile_id: seedData.agentProfileId,
        pr_number: 55,
        pr_branch: "refactor/auth",
        pr_repo: "testorg/testrepo",
        pr_author: "contributor",
      },
    });

    // --- Navigate to kanban and verify task in Waiting ---
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardInColumn("PR #55: Refactor auth", waitingStep.id);
    await expect(card).toBeVisible({ timeout: 15_000 });

    // --- Open task → creates CREATED session (workspace-only preparation) ---
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Wait for the session to be created (auto-prepare fires from useAutoStartSession)
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.length > 0 && sessions[0]?.state === "CREATED";
        },
        { timeout: 30_000, message: "Waiting for session to be created in CREATED state" },
      )
      .toBe(true);

    // --- Go back to kanban ---
    await kanban.goto();
    await expect(kanban.taskCardByTitle("PR #55: Refactor auth")).toBeVisible({ timeout: 15_000 });

    // --- Move task from Waiting to Review (triggers on_enter: auto_start_agent) ---
    await apiClient.moveTask(task.id, workflow.id, reviewStep.id);

    // --- Task should move through Review to Done after agent completes ---
    // If the bug is present, the prompt would be queued and the agent never starts,
    // so the task would stay in Review indefinitely.
    const cardInDone = kanban.taskCardInColumn("PR #55: Refactor auth", doneStep.id);
    await expect(cardInDone).toBeVisible({ timeout: 60_000 });

    // --- Navigate to session and verify agent completed (not stuck with queued prompt) ---
    await cardInDone.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const sessionAfter = new SessionPage(testPage);
    await sessionAfter.waitForLoad();

    // The agent should have completed — idle input visible
    await expect(sessionAfter.idleInput()).toBeVisible({ timeout: 30_000 });
  });
});
