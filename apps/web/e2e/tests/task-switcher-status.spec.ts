import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";

test.describe("Task switcher sidebar status", () => {
  /**
   * Verifies that the sidebar task switcher reflects real-time status updates
   * for tasks the user hasn't clicked, and that status is preserved correctly
   * when switching between tasks.
   *
   * This tests the fix for stale sidebar status: without the fix,
   * session.state_changed WS events were only sent to the active session's
   * subscribers. Tasks completing in the background would show outdated
   * status until the user clicked them to trigger a fresh fetch.
   *
   * Setup:
   *   Inbox → Working (auto_start, message+15s delay, on_turn_complete → Done) → Done
   *   3 tasks in Inbox.
   *
   * The mock agent sends a message immediately (triggers RUNNING / "In Progress"),
   * then delays 15s (stays in RUNNING), then completes (→ WAITING_FOR_INPUT / "Review").
   * Each task progresses through: Backlog → In Progress → Review.
   */
  test("sidebar reflects real-time status updates and preserves state on task switch", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    // --- Seed workflow ---
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Switcher Status Workflow",
    );

    const inboxStep = await apiClient.createWorkflowStep(workflow.id, "Inbox", 0);
    const workingStep = await apiClient.createWorkflowStep(workflow.id, "Working", 1);
    const doneStep = await apiClient.createWorkflowStep(workflow.id, "Done", 2);

    // message first (triggers RUNNING), delay 15s, then complete
    await apiClient.updateWorkflowStep(workingStep.id, {
      prompt:
        'e2e:message("working...")\ne2e:delay(15000)\ne2e:message("task finished")\n{{task_prompt}}',
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

    // --- Seed 3 tasks ---
    const taskAlpha = await apiClient.createTask(seedData.workspaceId, "Alpha Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });

    const taskBeta = await apiClient.createTask(seedData.workspaceId, "Beta Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });

    const taskGamma = await apiClient.createTask(seedData.workspaceId, "Gamma Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });

    // --- Complete Alpha first, then navigate to its session ---
    await apiClient.moveTask(taskAlpha.id, workflow.id, workingStep.id);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const alphaInDone = kanban.taskCardInColumn("Alpha Task", doneStep.id);
    await expect(alphaInDone).toBeVisible({ timeout: 45_000 });

    await alphaInDone.click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // --- Initial state: Alpha in Review, Beta + Gamma in Backlog ---
    await expect(session.taskInSection("Alpha Task", "Review")).toBeVisible({ timeout: 15_000 });
    await expect(session.taskInSection("Beta Task", "Backlog")).toBeVisible({ timeout: 10_000 });
    await expect(session.taskInSection("Gamma Task", "Backlog")).toBeVisible({ timeout: 10_000 });

    // --- Move Beta to Working (background task) ---
    await apiClient.moveTask(taskBeta.id, workflow.id, workingStep.id);

    // Beta should appear in "In Progress" (RUNNING state) while the 15s delay runs
    await expect(session.taskInSection("Beta Task", "In Progress")).toBeVisible({
      timeout: 30_000,
    });

    // While Beta is in progress, Alpha should remain in Review and Gamma in Backlog
    await expect(session.taskInSection("Alpha Task", "Review")).toBeVisible({ timeout: 5_000 });
    await expect(session.taskInSection("Gamma Task", "Backlog")).toBeVisible({ timeout: 5_000 });

    // Beta transitions from "In Progress" → "Review" after the 15s delay completes
    await expect(session.taskInSection("Beta Task", "Review")).toBeVisible({ timeout: 30_000 });

    // --- Move Gamma to Working (background task) ---
    // Small delay to let the backend settle after Beta's completion
    await testPage.waitForTimeout(2_000);
    await apiClient.moveTask(taskGamma.id, workflow.id, workingStep.id);

    // Gamma should appear in "In Progress" while running
    await expect(session.taskInSection("Gamma Task", "In Progress")).toBeVisible({
      timeout: 45_000,
    });

    // Gamma transitions to "Review"
    await expect(session.taskInSection("Gamma Task", "Review")).toBeVisible({ timeout: 30_000 });

    // --- Switch to Beta's session by clicking in sidebar ---
    await session.taskInSidebar("Beta Task").click();

    // All three tasks should remain in "Review" after switching sessions
    await expect(session.taskInSection("Alpha Task", "Review")).toBeVisible({ timeout: 10_000 });
    await expect(session.taskInSection("Beta Task", "Review")).toBeVisible({ timeout: 10_000 });
    await expect(session.taskInSection("Gamma Task", "Review")).toBeVisible({ timeout: 10_000 });
  });
});
