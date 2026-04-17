import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

async function createProfiles(
  apiClient: InstanceType<typeof import("../../helpers/api-client").ApiClient>,
) {
  const { agents } = await apiClient.listAgents();
  if (agents.length === 0) throw new Error("no agents available in test fixtures");
  const agentId = agents[0].id;
  const profileA = await apiClient.createAgentProfile(agentId, "Profile A (fast)", {
    model: "mock-fast",
  });
  const profileB = await apiClient.createAgentProfile(agentId, "Profile B (slow)", {
    model: "mock-slow",
  });
  return { agentId, profileA, profileB };
}

async function pollSessions(
  apiClient: InstanceType<typeof import("../../helpers/api-client").ApiClient>,
  taskId: string,
  expectedCount: number,
  timeoutMs = 30_000,
) {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    const { sessions } = await apiClient.listTaskSessions(taskId);
    if (sessions.length >= expectedCount) return sessions;
    await new Promise((r) => setTimeout(r, 500));
  }
  const { sessions } = await apiClient.listTaskSessions(taskId);
  return sessions;
}

test.describe("Workflow agent profile switching", () => {
  test("manual step move creates new session with step's agent profile", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const { profileA, profileB } = await createProfiles(apiClient);

    // Create workflow: Inbox → Step1 (profileA, auto_start) → Step2 (profileB, auto_start) → Done
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Agent Switch Manual");
    const inbox = await apiClient.createWorkflowStep(workflow.id, "Inbox", 0);
    const step1 = await apiClient.createWorkflowStep(workflow.id, "Step1", 1);
    const step2 = await apiClient.createWorkflowStep(workflow.id, "Step2", 2);
    await apiClient.createWorkflowStep(workflow.id, "Done", 3);

    await apiClient.updateWorkflowStep(step1.id, {
      agent_profile_id: profileA.id,
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });
    await apiClient.updateWorkflowStep(step2.id, {
      agent_profile_id: profileB.id,
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });

    // Create task in Inbox (no auto_start)
    const task = await apiClient.createTask(seedData.workspaceId, "Manual Switch Task", {
      workflow_id: workflow.id,
      workflow_step_id: inbox.id,
      agent_profile_id: profileA.id,
      repository_ids: [seedData.repositoryId],
    });

    // Move to Step1 — triggers auto_start with profileA
    await apiClient.moveTask(task.id, workflow.id, step1.id);

    // Wait for first session
    const initialSessions = await pollSessions(apiClient, task.id, 1);
    expect(initialSessions.length).toBeGreaterThanOrEqual(1);
    expect(initialSessions[0].agent_profile_id).toBe(profileA.id);

    // Wait for agent to be ready before moving
    await new Promise((r) => setTimeout(r, 3000));

    // Move task to Step2 — should create new session with profileB
    await apiClient.moveTask(task.id, workflow.id, step2.id);

    // Poll for second session
    const finalSessions = await pollSessions(apiClient, task.id, 2);
    expect(finalSessions.length).toBeGreaterThanOrEqual(2);

    // Sort by started_at to get chronological order
    finalSessions.sort((a, b) => a.started_at.localeCompare(b.started_at));

    // First session should be profileA (completed), second should be profileB
    expect(finalSessions[0].agent_profile_id).toBe(profileA.id);
    expect(finalSessions[1].agent_profile_id).toBe(profileB.id);
  });

  test("on_turn_complete transition creates new session with next step's agent profile", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const { profileA, profileB } = await createProfiles(apiClient);

    // Create workflow: Inbox → Step1 (profileA, auto_start, move_to_next) → Step2 (profileB, auto_start) → Done
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Agent Switch Auto");
    const inbox = await apiClient.createWorkflowStep(workflow.id, "Inbox", 0);
    const step1 = await apiClient.createWorkflowStep(workflow.id, "Step1", 1);
    const step2 = await apiClient.createWorkflowStep(workflow.id, "Step2", 2);
    await apiClient.createWorkflowStep(workflow.id, "Done", 3);

    await apiClient.updateWorkflowStep(step1.id, {
      agent_profile_id: profileA.id,
      prompt: 'e2e:delay(1000)\ne2e:message("step1 done")',
      events: {
        on_enter: [{ type: "auto_start_agent" }],
        on_turn_complete: [{ type: "move_to_next" }],
      },
    });
    await apiClient.updateWorkflowStep(step2.id, {
      agent_profile_id: profileB.id,
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });

    // Create task in Inbox
    const task = await apiClient.createTask(seedData.workspaceId, "Auto Switch Task", {
      workflow_id: workflow.id,
      workflow_step_id: inbox.id,
      agent_profile_id: profileA.id,
      repository_ids: [seedData.repositoryId],
    });

    // Move to Step1 — triggers auto_start with profileA, then on_turn_complete → Step2
    await apiClient.moveTask(task.id, workflow.id, step1.id);

    // Poll for second session (Step2 with profileB)
    const finalSessions = await pollSessions(apiClient, task.id, 2, 45_000);
    expect(finalSessions.length).toBeGreaterThanOrEqual(2);

    finalSessions.sort((a, b) => a.started_at.localeCompare(b.started_at));

    expect(finalSessions[0].agent_profile_id).toBe(profileA.id);
    expect(finalSessions[1].agent_profile_id).toBe(profileB.id);
  });

  test("reset context checkbox is disabled when step has agent profile override", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { profileA } = await createProfiles(apiClient);
    const stepId = seedData.steps[0].id;

    try {
      // Ensure clean state
      await apiClient.updateWorkflowStep(stepId, { agent_profile_id: "" });

      const page = new WorkflowSettingsPage(testPage);
      await page.goto(seedData.workspaceId);

      const card = await page.findWorkflowCard("E2E Workflow");
      await expect(card).toBeVisible();

      // Click first step to open config panel
      const stepNodes = card.locator(".group.relative");
      await stepNodes.first().click();
      await testPage.waitForTimeout(500);

      // Reset context checkbox should be enabled (no agent profile set)
      const resetCheckbox = card.getByRole("checkbox", { name: "Reset agent context" });
      await expect(resetCheckbox).toBeEnabled();

      // Set an agent profile on this step via API
      await apiClient.updateWorkflowStep(stepId, { agent_profile_id: profileA.id });

      // Reload and re-open the step
      await page.goto(seedData.workspaceId);
      const reloadedCard = await page.findWorkflowCard("E2E Workflow");
      const reloadedSteps = reloadedCard.locator(".group.relative");
      await reloadedSteps.first().click();
      await testPage.waitForTimeout(500);

      // Reset context checkbox should be disabled
      const reloadedCheckbox = reloadedCard.getByRole("checkbox", {
        name: "Reset agent context",
      });
      await expect(reloadedCheckbox).toBeDisabled();
    } finally {
      // Always clean up the seeded step to avoid leaking into other tests
      await apiClient.updateWorkflowStep(stepId, { agent_profile_id: "" });
    }
  });

  // Regression test for the frontend fix in agent-session.ts — when a
  // workflow step transition creates a new session with a different agent
  // profile, the chat UI must follow the switch. Without the fix, the chat
  // input stays bound to the first session and messages go to the wrong
  // agent even though the backend correctly spawned a new session.
  test("chat UI follows step transition to new session", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { profileA, profileB } = await createProfiles(apiClient);

    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Agent Switch UI");
    const inbox = await apiClient.createWorkflowStep(workflow.id, "Inbox", 0);
    const step1 = await apiClient.createWorkflowStep(workflow.id, "Step1", 1);
    const step2 = await apiClient.createWorkflowStep(workflow.id, "Step2", 2);
    await apiClient.createWorkflowStep(workflow.id, "Done", 3);

    // Give each step a mock prompt so the agent produces an observable
    // response and reaches an idle state — without this, the session stays
    // STARTING/RUNNING indefinitely and downstream UI assertions race.
    await apiClient.updateWorkflowStep(step1.id, {
      agent_profile_id: profileA.id,
      prompt: 'e2e:message("step1 ready")',
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });
    await apiClient.updateWorkflowStep(step2.id, {
      agent_profile_id: profileB.id,
      prompt: 'e2e:message("step2 ready")',
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });

    // Point the kanban at our workflow so we can find the task card there.
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      enable_preview_on_click: false,
    });

    const task = await apiClient.createTask(seedData.workspaceId, "UI Switch Task", {
      workflow_id: workflow.id,
      workflow_step_id: inbox.id,
      agent_profile_id: profileA.id,
      repository_ids: [seedData.repositoryId],
    });

    // Move to Step1 — auto_start creates session A with profileA.
    await apiClient.moveTask(task.id, workflow.id, step1.id);

    // Navigate via kanban → click card (matches the working workflow-automation
    // test) so activeTaskId is set by the normal app flow.
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const card = kanban.taskCardInColumn("UI Switch Task", step1.id);
    await expect(card).toBeVisible({ timeout: 15_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    // Wait for session A to respond — confirms the agent is connected and the
    // chat panel is fully wired before we trigger the next transition.
    await expect(session.chat.getByText("step1 ready", { exact: true })).toBeVisible({
      timeout: 45_000,
    });

    const initial = await pollSessions(apiClient, task.id, 1);
    const sessionA = initial.find((s) => s.agent_profile_id === profileA.id);
    expect(sessionA, "expected a session with profileA to be created").toBeDefined();

    // Move to Step2 — backend spawns session B with profileB. The frontend
    // fix is what makes the chat UI follow: without it, `useAutoSessionTab`
    // never fires for session B because `activeSessionId` stays pinned to
    // session A, so session B's tab is never added to dockview and the
    // assertion below times out.
    await apiClient.moveTask(task.id, workflow.id, step2.id);

    const afterMove = await pollSessions(apiClient, task.id, 2, 45_000);
    const sessionB = afterMove.find((s) => s.agent_profile_id === profileB.id);
    expect(sessionB, "expected a second session with profileB after moving to Step2").toBeDefined();

    await expect(session.sessionTabBySessionId(sessionB!.id)).toBeVisible({ timeout: 30_000 });
  });
});
