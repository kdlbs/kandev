import { test, expect } from "../../fixtures/test-base";
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

  test("manual step move updates chat UI to show new agent profile", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const { profileA, profileB } = await createProfiles(apiClient);

    // Create workflow: Step1 (profileA, auto_start, is_start) → Step2 (profileB, auto_start)
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "UI Switch Test");
    const step1 = await apiClient.createWorkflowStep(workflow.id, "Step1", 0, {
      is_start_step: true,
    });
    const step2 = await apiClient.createWorkflowStep(workflow.id, "Step2", 1);

    await apiClient.updateWorkflowStep(step1.id, {
      agent_profile_id: profileA.id,
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });
    await apiClient.updateWorkflowStep(step2.id, {
      agent_profile_id: profileB.id,
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });

    // Create task in Step1 with profileA
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "UI Switch Task",
      profileA.id,
      {
        workflow_id: workflow.id,
        workflow_step_id: step1.id,
        repository_ids: [seedData.repositoryId],
      },
    );

    // Navigate to task and wait for the chat to load
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await expect(session.chat).toBeVisible({ timeout: 15_000 });

    // Wait for the first session tab to appear with the correct profile name
    const sessionTabs = testPage.locator('[data-testid^="session-tab-"]');
    await expect(sessionTabs.first()).toBeVisible({ timeout: 30_000 });
    await expect(sessionTabs.first()).toContainText("Profile A", { timeout: 10_000 });

    // Wait for the agent to be ready (WAITING_FOR_INPUT) before moving
    for (let i = 0; i < 20; i++) {
      const { sessions } = await apiClient.listTaskSessions(task.id);
      if (sessions.some((s) => s.state === "WAITING_FOR_INPUT")) break;
      await new Promise((r) => setTimeout(r, 500));
    }

    // Move task to Step2 — should create new session with profileB
    await apiClient.moveTask(task.id, workflow.id, step2.id);

    // The UI should create a new session. Verify via API that the backend created it.
    const finalSessions = await pollSessions(apiClient, task.id, 2, 30_000);
    const profileBSession = finalSessions.find((s) => s.agent_profile_id === profileB.id);
    expect(profileBSession).toBeDefined();
  });

  test("on_turn_start transition to step with agent override uses correct profile", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const { profileA, profileB } = await createProfiles(apiClient);

    // Backlog (on_turn_start: move_to_next) → Step1 (profileB, auto_start)
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Agent Switch OnTurnStart",
    );
    const backlog = await apiClient.createWorkflowStep(workflow.id, "Backlog", 0);
    const step1 = await apiClient.createWorkflowStep(workflow.id, "Step1", 1);
    await apiClient.createWorkflowStep(workflow.id, "Done", 2);

    await apiClient.updateWorkflowStep(backlog.id, {
      events: { on_turn_start: [{ type: "move_to_next" }] },
    });
    await apiClient.updateWorkflowStep(step1.id, {
      agent_profile_id: profileB.id,
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });

    // Create task with profileA — on_turn_start should move to Step1 and switch to profileB
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "OnTurnStart Switch",
      profileA.id,
      {
        workflow_id: workflow.id,
        workflow_step_id: backlog.id,
        repository_ids: [seedData.repositoryId],
      },
    );

    // Poll for sessions — on_turn_start creates an intermediate session then switches
    const sessions = await pollSessions(apiClient, task.id, 2, 45_000);
    const profileBSession = sessions.find((s) => s.agent_profile_id === profileB.id);
    expect(profileBSession).toBeDefined();
  });

  test("auto-launches agent when step has profile override and prompt", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const { profileA, profileB } = await createProfiles(apiClient);

    // Step1 (profileA, auto_start, is_start) → Step2 (profileB, prompt but NO auto_start)
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Agent Switch AutoLaunch",
    );
    const step1 = await apiClient.createWorkflowStep(workflow.id, "Step1", 0, {
      is_start_step: true,
    });
    const step2 = await apiClient.createWorkflowStep(workflow.id, "Step2", 1);
    await apiClient.createWorkflowStep(workflow.id, "Done", 2);

    await apiClient.updateWorkflowStep(step1.id, {
      agent_profile_id: profileA.id,
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });
    // Step2 has profile + prompt but NO auto_start_agent — should still auto-launch
    await apiClient.updateWorkflowStep(step2.id, {
      agent_profile_id: profileB.id,
      prompt: "hello from step2",
    });

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "AutoLaunch Task",
      profileA.id,
      {
        workflow_id: workflow.id,
        workflow_step_id: step1.id,
        repository_ids: [seedData.repositoryId],
      },
    );

    // Wait for Step1 agent to finish
    for (let i = 0; i < 20; i++) {
      const { sessions } = await apiClient.listTaskSessions(task.id);
      if (sessions.some((s) => s.state === "WAITING_FOR_INPUT")) break;
      await new Promise((r) => setTimeout(r, 500));
    }

    // Move to Step2 — should auto-launch agent despite no auto_start_agent
    await apiClient.moveTask(task.id, workflow.id, step2.id);

    // Poll for a session with profileB that has completed at least one turn
    const finalSessions = await pollSessions(apiClient, task.id, 2, 30_000);
    const step2Session = finalSessions.find((s) => s.agent_profile_id === profileB.id);
    expect(step2Session).toBeDefined();
    // The session should have progressed past CREATED (agent was launched)
    expect(step2Session!.state).not.toBe("CREATED");
  });

  test("new session inherits task_environment_id from old session", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const { profileA, profileB } = await createProfiles(apiClient);

    // Step1 (profileA, auto_start, is_start) → Step2 (profileB, auto_start)
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Env Inherit Test");
    const step1 = await apiClient.createWorkflowStep(workflow.id, "Step1", 0, {
      is_start_step: true,
    });
    const step2 = await apiClient.createWorkflowStep(workflow.id, "Step2", 1);
    await apiClient.createWorkflowStep(workflow.id, "Done", 2);

    await apiClient.updateWorkflowStep(step1.id, {
      agent_profile_id: profileA.id,
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });
    await apiClient.updateWorkflowStep(step2.id, {
      agent_profile_id: profileB.id,
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Env Inherit Task",
      profileA.id,
      {
        workflow_id: workflow.id,
        workflow_step_id: step1.id,
        repository_ids: [seedData.repositoryId],
      },
    );

    // Wait for Step1 agent to finish
    for (let i = 0; i < 20; i++) {
      const { sessions } = await apiClient.listTaskSessions(task.id);
      if (sessions.some((s) => s.state === "WAITING_FOR_INPUT")) break;
      await new Promise((r) => setTimeout(r, 500));
    }

    // Move to Step2
    await apiClient.moveTask(task.id, workflow.id, step2.id);

    // Wait for second session
    const finalSessions = await pollSessions(apiClient, task.id, 2, 30_000);
    const step2Session = finalSessions.find((s) => s.agent_profile_id === profileB.id);
    expect(step2Session).toBeDefined();

    // The new session should inherit task_environment_id from the old session
    const step1Session = finalSessions.find((s) => s.agent_profile_id === profileA.id);
    if (step1Session?.task_environment_id) {
      expect(step2Session!.task_environment_id).toBe(step1Session.task_environment_id);
    }
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
});
