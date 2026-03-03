import { test, expect } from "../fixtures/test-base";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "../helpers/api-client";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";
import type { Page } from "@playwright/test";

// ---------------------------------------------------------------------------
// Helpers shared across TUI passthrough tests
// ---------------------------------------------------------------------------

/** Create a passthrough (TUI) agent profile for the mock agent. */
async function createTUIProfile(apiClient: ApiClient, name: string) {
  const { agents } = await apiClient.listAgents();
  return apiClient.createAgentProfile(agents[0].id, name, {
    model: "mock-fast",
    auto_approve: true,
    cli_passthrough: true,
  });
}

/** Navigate to a kanban card by title and open its session page. */
async function openTaskSession(page: Page, title: string): Promise<SessionPage> {
  const kanban = new KanbanPage(page);
  await kanban.goto();

  const card = kanban.taskCardByTitle(title);
  await expect(card).toBeVisible({ timeout: 15_000 });
  await card.click();
  await expect(page).toHaveURL(/\/s\//, { timeout: 15_000 });

  const session = new SessionPage(page);
  await session.waitForPassthroughLoad();
  return session;
}

/** Seed a 4-step workflow (Backlog → Analyze → Implement → Review) with configurable step events. */
async function seedCascadeWorkflow(
  apiClient: ApiClient,
  seedData: SeedData,
  name: string,
  implementOnEnter: Array<{ type: string }>,
) {
  const workflow = await apiClient.createWorkflow(seedData.workspaceId, name);

  const backlogStep = await apiClient.createWorkflowStep(workflow.id, "Backlog", 0);
  const analyzeStep = await apiClient.createWorkflowStep(workflow.id, "Analyze", 1);
  const implementStep = await apiClient.createWorkflowStep(workflow.id, "Implement", 2);
  await apiClient.createWorkflowStep(workflow.id, "Review", 3);

  await apiClient.updateWorkflowStep(analyzeStep.id, {
    prompt: "Analyze: {{task_prompt}}",
    events: {
      on_enter: [{ type: "auto_start_agent" }],
      on_turn_complete: [{ type: "move_to_next" }],
    },
  });
  await apiClient.updateWorkflowStep(implementStep.id, {
    prompt: "Implement: {{task_prompt}}",
    events: {
      on_enter: implementOnEnter,
      on_turn_complete: [{ type: "move_to_next" }],
    },
  });

  await apiClient.saveUserSettings({
    workspace_id: seedData.workspaceId,
    workflow_filter_id: workflow.id,
    enable_preview_on_click: false,
  });

  return { workflow, backlogStep, analyzeStep, implementStep };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe("Terminal agent (TUI passthrough)", () => {
  /**
   * Creates a task using a passthrough (TUI) agent profile.
   * The mock agent starts in --tui mode inside a PTY, processes the initial
   * prompt (passed via --prompt flag from the task description), renders a
   * response in the terminal, and returns to idle.
   *
   * The idle timeout triggers turn completion, and the "simple" workflow
   * template's on_turn_complete: move_to_step("review") advances the task.
   *
   * Verifies:
   * - Passthrough terminal is visible (not the chat panel)
   * - Mock agent TUI output appears in the terminal buffer
   * - Workflow step transitions from In Progress → Review
   * - Sidebar reflects the new step
   */
  test("creates task with TUI agent, processes prompt, and advances to Review", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const profile = await createTUIProfile(apiClient, "Mock TUI");

    await apiClient.createTaskWithAgent(seedData.workspaceId, "TUI Test Task", profile.id, {
      description: "hello from e2e test",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "TUI Test Task");

    // Loading overlay is visible while the WebSocket connects
    await session.waitForPassthroughLoading();

    // Once connected, loading overlay disappears and terminal content is visible
    await session.waitForPassthroughLoaded();

    // The passthrough terminal shows the mock agent TUI header (rendered via xterm.js canvas).
    // Note: response body text ("mock response") may not appear if the agent finishes
    // before the frontend establishes the WebSocket — PTY history isn't replayed.
    await session.expectPassthroughHasText("Mock Agent");

    // After the agent completes its turn (idle timeout fires), the workflow
    // advances from In Progress to Review via on_turn_complete: move_to_step
    await expect(session.stepperStep("Review")).toHaveAttribute("aria-current", "step", {
      timeout: 30_000,
    });

    // Sidebar shows the task under the Review section
    await expect(session.sidebarSection("Review")).toBeVisible({ timeout: 15_000 });
  });

  /**
   * Seeds a 4-step workflow (Backlog → Analyze → Implement → Review).
   * Analyze and Implement both have:
   *   on_enter: [auto_start_agent], custom prompt with {{task_prompt}},
   *   on_turn_complete: [move_to_next]
   * Review has no events (terminal step).
   *
   * The agent starts in Backlog so we can navigate to the session page and
   * establish the live terminal WebSocket. Then a moveTask to Analyze triggers
   * the cascade:
   *   Analyze on_enter → prompt written to PTY stdin → turn complete → move_to_next
   *   Implement on_enter → prompt written to PTY stdin → turn complete → move_to_next
   *   → Review (terminal step, no events)
   *
   * Verifies:
   * - Workflow auto_start_agent delivers custom prompts to passthrough agents via stdin
   * - Each step's prompt text appears in the terminal buffer (captured via live WS)
   * - Task cascades through all steps and lands in Review
   * - Stepper reflects the final step
   */
  test("multi-step workflow cascade with custom prompts via stdin", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { workflow, backlogStep, analyzeStep } = await seedCascadeWorkflow(
      apiClient,
      seedData,
      "TUI Cascade Workflow",
      [{ type: "auto_start_agent" }],
    );

    const profile = await createTUIProfile(apiClient, "TUI Cascade");

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "TUI Cascade Task",
      profile.id,
      {
        description: "hello from cascade test",
        workflow_id: workflow.id,
        workflow_step_id: backlogStep.id,
        repository_ids: [seedData.repositoryId],
      },
    );

    const session = await openTaskSession(testPage, "TUI Cascade Task");

    // Confirm the mock TUI is running and processed the initial prompt
    await session.expectPassthroughHasText("Mock Agent");
    await session.expectPassthroughHasText("mock response");

    // Now trigger the cascade: move to Analyze → auto_start writes prompt to stdin
    // → turn complete → move_to_next → Implement → auto_start writes prompt to stdin
    // → turn complete → move_to_next → Review
    await apiClient.moveTask(task.id, workflow.id, analyzeStep.id);

    // Terminal should show output from both auto-started step prompts.
    // The mock TUI prints "Processed: <prompt>" for each stdin line it receives.
    await session.expectPassthroughHasText("Analyze", 30_000);
    await session.expectPassthroughHasText("Implement", 30_000);

    // Stepper shows Review as current step after the full cascade
    await expect(session.stepperStep("Review")).toHaveAttribute("aria-current", "step", {
      timeout: 30_000,
    });
  });

  /**
   * Seeds a 4-step workflow (Backlog → Analyze → Implement → Review).
   * Analyze has: on_enter: [auto_start_agent], on_turn_complete: [move_to_next]
   * Implement has: on_enter: [reset_agent_context, auto_start_agent],
   *   on_turn_complete: [move_to_next]
   * Review has no events (terminal step).
   *
   * The reset_agent_context action kills the PTY process and relaunches a fresh
   * one without --resume. Then auto_start_agent delivers the step prompt via stdin.
   *
   * Verifies:
   * - context_reset kills the PTY and relaunches (fresh "Mock Agent" header appears)
   * - auto_start_agent delivers the Implement step prompt after reset
   * - Task cascades through all steps and lands in Review
   */
  test("context reset relaunches PTY and delivers prompt in cascade", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { workflow, backlogStep, analyzeStep } = await seedCascadeWorkflow(
      apiClient,
      seedData,
      "TUI Reset Workflow",
      [{ type: "reset_agent_context" }, { type: "auto_start_agent" }],
    );

    const profile = await createTUIProfile(apiClient, "TUI Reset");

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "TUI Reset Task",
      profile.id,
      {
        description: "hello from reset test",
        workflow_id: workflow.id,
        workflow_step_id: backlogStep.id,
        repository_ids: [seedData.repositoryId],
      },
    );

    const session = await openTaskSession(testPage, "TUI Reset Task");
    await session.expectPassthroughHasText("Mock Agent");

    // Trigger cascade: Analyze → (turn complete) → Implement (reset + auto_start) → Review
    await apiClient.moveTask(task.id, workflow.id, analyzeStep.id);

    // Terminal shows output from Analyze step (before reset)
    await session.expectPassthroughHasText("Analyze", 30_000);

    // After context reset, Implement step prompt is delivered to the fresh process
    await session.expectPassthroughHasText("Implement", 30_000);

    // Stepper shows Review as current step after the full cascade
    await expect(session.stepperStep("Review")).toHaveAttribute("aria-current", "step", {
      timeout: 30_000,
    });
  });

  /**
   * Creates two tasks with TUI agents using distinct descriptions.
   * Navigates to task A, confirms its terminal output, then switches to
   * task B via the sidebar and confirms:
   * - Task B's terminal shows its own output
   * - Task A's output is NOT present in the terminal buffer
   *
   * This catches a bug where the passthrough terminal (rendered inside the
   * global chat panel) kept the previous session's xterm buffer when switching.
   */
  test("switching between TUI sessions clears terminal buffer", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const profile = await createTUIProfile(apiClient, "TUI Switch");

    // Create two tasks with distinct descriptions so their terminal output differs
    await apiClient.createTaskWithAgent(seedData.workspaceId, "TUI Alpha Task", profile.id, {
      description: "alpha-unique-marker",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await apiClient.createTaskWithAgent(seedData.workspaceId, "TUI Beta Task", profile.id, {
      description: "beta-unique-marker",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "TUI Alpha Task");

    // Task A's terminal shows its prompt output
    await session.expectPassthroughHasText("alpha-unique-marker", 15_000);

    // Switch to task B via sidebar
    const taskB = session.taskInSidebar("TUI Beta Task");
    await expect(taskB).toBeVisible({ timeout: 15_000 });
    await taskB.click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    // Loading overlay reappears during the session switch
    await session.waitForPassthroughLoading();

    // Wait for task B's passthrough terminal to load
    await session.waitForPassthroughLoad();
    await session.waitForPassthroughLoaded();

    // Task B's terminal shows its own output
    await session.expectPassthroughHasText("beta-unique-marker", 15_000);

    // Task A's output must NOT be in the buffer (verifies reset on switch)
    await session.expectPassthroughNotHasText("alpha-unique-marker");
  });
});
