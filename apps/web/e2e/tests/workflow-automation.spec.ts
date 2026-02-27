import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";

test.describe("Workflow automation", () => {
  /**
   * Seeds a 3-step workflow (Inbox → In Progress → Done).
   * Configures In Progress with on_enter: auto_start_agent and a custom step
   * prompt routed to the mock agent, plus on_turn_complete: move_to_step(Done).
   *
   * Verifies:
   * - auto_start_agent fires when task is moved into In Progress
   * - step custom prompt is used (not the task description)
   * - on_turn_complete advances task to Done column on the kanban board
   * - session stepper shows Done as the current step
   */
  test("auto_start_agent with step custom prompt; on_turn_complete advances to next column", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // --- Seed a workflow with explicit step configuration ---
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Automation Test Workflow",
    );

    const inboxStep = await apiClient.createWorkflowStep(workflow.id, "Inbox", 0);
    const inProgressStep = await apiClient.createWorkflowStep(workflow.id, "In Progress", 1);
    const doneStep = await apiClient.createWorkflowStep(workflow.id, "Done", 2);

    // Configure In Progress: auto_start_agent on enter, custom step prompt,
    // move to Done when agent turn completes.
    await apiClient.updateWorkflowStep(inProgressStep.id, {
      prompt: "/e2e:simple-message {{task_prompt}}",
      events: {
        on_enter: [{ type: "auto_start_agent" }],
        on_turn_complete: [{ type: "move_to_step", config: { step_id: doneStep.id } }],
      },
    });

    // Point the kanban page at our custom workflow
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      enable_preview_on_click: false,
    });

    // Resolve an agent profile so auto_start_agent can pick it up from task metadata
    const { agents } = await apiClient.listAgents();
    const agentProfileId = agents[0]?.profiles[0]?.id ?? "";

    // Create task in Inbox — does NOT trigger auto_start_agent
    const task = await apiClient.createTask(seedData.workspaceId, "Auto Agent Workflow Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: agentProfileId,
      repository_ids: [seedData.repositoryId],
    });

    // Move task to In Progress → triggers on_enter: auto_start_agent
    await apiClient.moveTask(task.id, workflow.id, inProgressStep.id);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // After the agent completes its turn, on_turn_complete fires and moves the
    // task to Done. The kanban board receives a WS push.
    const cardInDone = kanban.taskCardInColumn("Auto Agent Workflow Task", doneStep.id);
    await expect(cardInDone).toBeVisible({ timeout: 30_000 });

    // Navigate to the session page
    await cardInDone.click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Stepper shows Done as current step
    await expect(session.stepperStep("Done")).toHaveAttribute("aria-current", "step", {
      timeout: 10_000,
    });

    // Mock agent response confirms the step custom prompt was executed
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });
  });

  /**
   * Seeds a 4-step workflow (Todo → In Progress → Review → Done) and validates
   * the full lifecycle from both the kanban page and the session details page.
   *
   * Step configuration:
   * - Todo:        on_turn_complete: [move_to_next]
   * - In Progress: on_enter: [auto_start_agent], custom step prompt,
   *                on_turn_complete: [move_to_next]
   * - Review:      no events (manual step)
   * - Done:        no events
   *
   * Flow:
   * 1. Task is created in Todo
   * 2. Navigate to kanban → verify task is in Todo → click card to session page
   * 3. Send a message → agent responds → on_turn_complete fires → task moves to In Progress
   * 4. on_enter: auto_start_agent queues step prompt (/e2e:multi-turn)
   * 5. Step prompt runs → "Multi-turn response ready" → on_turn_complete → Review
   * 6. Stepper shows Review
   * 7. Send /e2e:simple-message in Review → "simple mock response" → stays in Review
   * 8. Move task to Done via API → stepper shows Done
   */
  test("full workflow lifecycle: kanban transitions and session stepper updates", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Lifecycle Test Workflow",
    );

    const todoStep = await apiClient.createWorkflowStep(workflow.id, "Todo", 0);
    const inProgressStep = await apiClient.createWorkflowStep(workflow.id, "In Progress", 1);
    const reviewStep = await apiClient.createWorkflowStep(workflow.id, "Review", 2);
    const doneStep = await apiClient.createWorkflowStep(workflow.id, "Done", 3);

    // Todo: on_turn_complete moves task to In Progress after the first agent turn.
    await apiClient.updateWorkflowStep(todoStep.id, {
      events: {
        on_turn_complete: [{ type: "move_to_next" }],
      },
    });

    // In Progress: auto_start_agent on enter with custom step prompt (multi-turn scenario),
    // move_to_next on turn complete.
    // Using /e2e:multi-turn (not simple-message) so the auto-start response is distinguishable
    // from the user's manually sent /e2e:simple-message later in the test.
    await apiClient.updateWorkflowStep(inProgressStep.id, {
      prompt: "/e2e:multi-turn {{task_prompt}}",
      events: {
        on_enter: [{ type: "auto_start_agent" }],
        on_turn_complete: [{ type: "move_to_next" }],
      },
    });

    // Review: no events — manual step (agent turns don't trigger transitions)
    // Done: no events (default, no update needed)

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      enable_preview_on_click: false,
    });

    const { agents } = await apiClient.listAgents();
    const agentProfileId = agents[0]?.profiles[0]?.id ?? "";

    // Create task in Todo without starting an agent.
    const task = await apiClient.createTask(seedData.workspaceId, "Lifecycle Workflow Task", {
      workflow_id: workflow.id,
      workflow_step_id: todoStep.id,
      agent_profile_id: agentProfileId,
      repository_ids: [seedData.repositoryId],
    });

    // --- Kanban page: verify task starts in Todo ---
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const cardInTodo = kanban.taskCardInColumn("Lifecycle Workflow Task", todoStep.id);
    await expect(cardInTodo).toBeVisible({ timeout: 10_000 });

    // Click the card — creates a session (with workflow_step_id from the task)
    // and navigates to the session page.
    await cardInTodo.click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Stepper initially shows Todo
    await expect(session.stepperStep("Todo")).toHaveAttribute("aria-current", "step", {
      timeout: 10_000,
    });

    // --- Send a message: agent responds → on_turn_complete fires → cascade ---
    // Todo on_turn_complete: move_to_next → In Progress
    // In Progress on_enter: auto_start_agent queues /e2e:multi-turn step prompt
    // Step prompt runs → on_turn_complete: move_to_next → Review
    await session.sendMessage("/e2e:simple-message");

    // Wait for the first mock response (from user's /e2e:simple-message on Todo step)
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });

    // The auto-start step prompt (/e2e:multi-turn) produces a distinct response
    await expect(session.chat.getByText("Multi-turn response ready", { exact: false })).toBeVisible(
      { timeout: 30_000 },
    );

    // Stepper should show Review after the full cascade completes
    await expect(session.stepperStep("Review")).toHaveAttribute("aria-current", "step", {
      timeout: 30_000,
    });

    // --- Send another message in Review: verify manual step stays ---
    await session.sendMessage("/e2e:simple-message");

    // Wait for the agent to respond (second "simple mock response")
    await expect(
      session.chat.getByText("simple mock response", { exact: false }).nth(1),
    ).toBeVisible({ timeout: 30_000 });

    // Stepper still shows Review (no on_turn_complete events on Review)
    await expect(session.stepperStep("Review")).toHaveAttribute("aria-current", "step", {
      timeout: 10_000,
    });

    // --- Move task to Done manually via API ---
    await apiClient.moveTask(task.id, workflow.id, doneStep.id);

    // Stepper updates to Done via WS push
    await expect(session.stepperStep("Done")).toHaveAttribute("aria-current", "step", {
      timeout: 10_000,
    });
  });

  /**
   * Seeds a 2-step workflow (Inbox → Planning).
   * Configures Planning with on_enter: [auto_start_agent, enable_plan_mode]
   * and a custom prompt. No on_turn_complete so task stays in Planning.
   *
   * Verifies:
   * - enable_plan_mode activates plan mode UI
   * - Plan mode chat input placeholder is visible in the session
   */
  test("enable_plan_mode event activates plan mode UI in session", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Plan Mode Test Workflow",
    );

    const inboxStep = await apiClient.createWorkflowStep(workflow.id, "Inbox", 0);
    const planningStep = await apiClient.createWorkflowStep(workflow.id, "Planning", 1);

    // Configure Planning: auto_start_agent + enable_plan_mode on enter.
    // No on_turn_complete so the task remains in Planning after the agent turn.
    await apiClient.updateWorkflowStep(planningStep.id, {
      prompt: "/e2e:simple-message {{task_prompt}}",
      events: {
        on_enter: [{ type: "auto_start_agent" }, { type: "enable_plan_mode" }],
      },
    });

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      enable_preview_on_click: false,
    });

    const { agents } = await apiClient.listAgents();
    const agentProfileId = agents[0]?.profiles[0]?.id ?? "";

    const task = await apiClient.createTask(seedData.workspaceId, "Plan Mode Workflow Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: agentProfileId,
      repository_ids: [seedData.repositoryId],
    });

    // Move task to Planning → triggers auto_start_agent + enable_plan_mode
    await apiClient.moveTask(task.id, workflow.id, planningStep.id);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // Task stays in Planning (no on_turn_complete configured)
    const card = kanban.taskCardInColumn("Plan Mode Workflow Task", planningStep.id);
    await expect(card).toBeVisible({ timeout: 30_000 });

    // Navigate to session page
    await card.click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Stepper shows Planning as current step
    await expect(session.stepperStep("Planning")).toHaveAttribute("aria-current", "step", {
      timeout: 10_000,
    });

    // Plan panel is visible — the layout switches to plan mode when the session
    // is created with enable_plan_mode.
    await expect(session.planPanel).toBeVisible({ timeout: 15_000 });

    // "Plan mode" badge on the message confirms plan mode was active for this session.
    // The badge appears when message.metadata.plan_mode = true, which the backend sets
    // when the session is auto-started via the enable_plan_mode workflow event.
    await expect(session.planModeBadge()).toBeVisible({ timeout: 15_000 });
  });

  /**
   * Seeds a 5-step workflow (Backlog → Analyze → Implement → Test → Deploy).
   * Steps Analyze, Implement, and Test each have:
   *   on_enter: [auto_start_agent], custom prompt with {{task_prompt}},
   *   on_turn_complete: [move_to_next]
   * Deploy has on_enter: [auto_start_agent] and a custom prompt but no on_turn_complete
   * (terminal step).
   *
   * A single moveTask(Backlog → Analyze) triggers a full cascade through all 4
   * active steps. Each step's custom prompt is sent to the mock agent, which
   * falls through to emitRandomResponse and echoes the prompt text back.
   *
   * Verifies:
   * - auto_start_agent fires on each step entry
   * - task cascades through all steps via on_turn_complete: move_to_next
   * - each step's agent response includes that step's custom prompt text
   * - the task description is included in every response (via {{task_prompt}})
   * - task lands in Deploy (terminal step) on the kanban board
   * - session stepper shows Deploy as the current step
   */
  test("multi-step cascade: each step uses its own custom prompt with task description", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const taskTitle = "Cascade Workflow Task";
    const taskDescription = "Build the login page";

    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Cascade Test Workflow");

    const backlogStep = await apiClient.createWorkflowStep(workflow.id, "Backlog", 0);
    const analyzeStep = await apiClient.createWorkflowStep(workflow.id, "Analyze", 1);
    const implementStep = await apiClient.createWorkflowStep(workflow.id, "Implement", 2);
    const testStep = await apiClient.createWorkflowStep(workflow.id, "Test", 3);
    const deployStep = await apiClient.createWorkflowStep(workflow.id, "Deploy", 4);

    // Each active step has a unique prompt prefix so we can assert per-step responses.
    // {{task_prompt}} is replaced with the task description by buildWorkflowPrompt.
    // Without a /e2e: prefix the mock agent falls to emitRandomResponse which echoes
    // the full prompt: "I've completed the analysis of your request: \"<prompt>\". ..."
    const stepConfigs = [
      { step: analyzeStep, prompt: "Analyze requirements for: {{task_prompt}}" },
      { step: implementStep, prompt: "Implement solution for: {{task_prompt}}" },
      { step: testStep, prompt: "Write tests for: {{task_prompt}}" },
      { step: deployStep, prompt: "Deploy changes for: {{task_prompt}}" },
    ];

    for (const { step, prompt } of stepConfigs) {
      const isTerminal = step.id === deployStep.id;
      await apiClient.updateWorkflowStep(step.id, {
        prompt,
        events: {
          on_enter: [{ type: "auto_start_agent" }],
          ...(isTerminal ? {} : { on_turn_complete: [{ type: "move_to_next" }] }),
        },
      });
    }

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      enable_preview_on_click: false,
    });

    const { agents } = await apiClient.listAgents();
    const agentProfileId = agents[0]?.profiles[0]?.id ?? "";

    // Create task in Backlog (staging step, no events).
    // The description (not title) is what {{task_prompt}} resolves to in step prompts.
    const task = await apiClient.createTask(seedData.workspaceId, taskTitle, {
      description: taskDescription,
      workflow_id: workflow.id,
      workflow_step_id: backlogStep.id,
      agent_profile_id: agentProfileId,
      repository_ids: [seedData.repositoryId],
    });

    // Move to Analyze → triggers on_enter: auto_start_agent → cascade begins
    await apiClient.moveTask(task.id, workflow.id, analyzeStep.id);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // After the full cascade, the task should land in Deploy (terminal step).
    const cardInDeploy = kanban.taskCardInColumn(taskTitle, deployStep.id);
    await expect(cardInDeploy).toBeVisible({ timeout: 60_000 });

    // Navigate to the session page to verify per-step responses
    await cardInDeploy.click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Stepper shows Deploy as current step
    await expect(session.stepperStep("Deploy")).toHaveAttribute("aria-current", "step", {
      timeout: 10_000,
    });

    // Each step's agent response echoes the full prompt via emitRandomResponse:
    // "I've completed the analysis of your request: \"<step prompt with task description>\". ..."
    // We match the echoed prompt inside the response text to confirm both the step-specific
    // prefix and the task description were received by the agent.
    const expectedResponseFragments = [
      `Analyze requirements for: ${taskDescription}`,
      `Implement solution for: ${taskDescription}`,
      `Write tests for: ${taskDescription}`,
      `Deploy changes for: ${taskDescription}`,
    ];

    for (const fragment of expectedResponseFragments) {
      await expect(session.chat.getByText(fragment, { exact: false }).first()).toBeVisible({
        timeout: 10_000,
      });
    }
  });
});
