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
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const { profileA, profileB } = await createProfiles(apiClient);

    // Backlog (on_turn_start: move_to_next) → Step1 (profileB)
    // Per PR #743, on_turn_start fires on user-message dispatch (not agent boot),
    // and on_turn_start transitions skip on_enter — the user's prompt is delivered
    // to the new step's profile via maybeSwitchSessionForProfile.
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
    });

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

    // Wait for profileA session to be ready before sending the message
    // that triggers on_turn_start.
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await expect(session.chat).toBeVisible({ timeout: 15_000 });
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.some((s) => s.state === "WAITING_FOR_INPUT");
        },
        { timeout: 30_000, message: "Waiting for profileA agent to be ready" },
      )
      .toBe(true);

    // User message → dispatchPromptAsync → on_turn_start → move_to_next → Step1 (profileB)
    await session.sendMessage("hello");
    await expect(session.chat.getByText("hello")).toBeVisible({ timeout: 10_000 });

    // Profile switch produces (or reuses) a session with profileB on this task.
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.some((s) => s.agent_profile_id === profileB.id);
        },
        { timeout: 30_000, message: "Waiting for profileB session after on_turn_start" },
      )
      .toBe(true);
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

    // Poll until a session with profileB progresses past CREATED (agent launched).
    // The agent launch is async (goroutine), so we need to wait for state progression.
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          const step2Session = sessions.find((s) => s.agent_profile_id === profileB.id);
          if (!step2Session || step2Session.state === "CREATED") return "CREATED";
          return step2Session.state;
        },
        { timeout: 30_000, message: "Waiting for step2 agent to launch past CREATED" },
      )
      .not.toBe("CREATED");
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

  /**
   * Verifies the StartCreatedSession fix: when a task is created in a step with an
   * agent profile override, the session tab shows the step's agent profile name
   * (not the workspace default).
   */
  test("initial task creation shows step's agent profile in session tab", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const { profileA } = await createProfiles(apiClient);

    // Create workflow with Step1 (profileA, auto_start, is_start)
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Initial Profile Tab");
    const step1 = await apiClient.createWorkflowStep(workflow.id, "Step1", 0, {
      is_start_step: true,
    });
    await apiClient.createWorkflowStep(workflow.id, "Done", 1);

    await apiClient.updateWorkflowStep(step1.id, {
      agent_profile_id: profileA.id,
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });

    // Create task — should use Step1's profileA, not the workspace default
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Initial Profile Task",
      profileA.id,
      {
        workflow_id: workflow.id,
        workflow_step_id: step1.id,
        repository_ids: [seedData.repositoryId],
      },
    );

    // Navigate to task
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await expect(session.chat).toBeVisible({ timeout: 15_000 });

    // The session tab should show "Profile A" (the step's agent profile name)
    const sessionTab = testPage.locator('[data-testid^="session-tab-"]').first();
    await expect(sessionTab).toBeVisible({ timeout: 30_000 });
    await expect(sessionTab).toContainText("Profile A", { timeout: 10_000 });
  });

  /**
   * Verifies the switchSessionForStep WS event fix: when a task is manually moved
   * to a step with a different agent profile, the new session is created and
   * promoted to primary.
   *
   * Note: Per PR #743 (pinnedSessionId), navigating to /t/<taskId> calls
   * setActiveSession which pins the initial primary. Workflow profile switches
   * no longer yank focus from a pinned session, so this test asserts on the
   * primary star indicator (not the active dockview tab).
   */
  test("manual step move promotes new agent session to primary", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const { profileA, profileB } = await createProfiles(apiClient);

    // Step1 (profileA, auto_start, is_start) → Step2 (profileB, auto_start)
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Active Tab Switch");
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
      "Tab Switch Task",
      profileA.id,
      {
        workflow_id: workflow.id,
        workflow_step_id: step1.id,
        repository_ids: [seedData.repositoryId],
      },
    );

    // Navigate and wait for first session
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await expect(session.chat).toBeVisible({ timeout: 15_000 });

    // Wait for Profile A tab to appear and be active
    const profileATab = session.sessionTabByText("Profile A");
    await expect(profileATab).toBeVisible({ timeout: 30_000 });

    // Wait for agent to be ready (WAITING_FOR_INPUT)
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.some((s) => s.state === "WAITING_FOR_INPUT");
        },
        { timeout: 30_000, message: "Waiting for agent to be ready" },
      )
      .toBe(true);

    // Move to Step2 — triggers new session with profileB
    await apiClient.moveTask(task.id, workflow.id, step2.id);

    // The new "Profile B" tab should appear and the primary star should move to it
    // (the active tab stays on Profile A because the SSR-load pin protects it).
    const profileBTab = session.sessionTabByText("Profile B");
    await expect(profileBTab).toBeVisible({ timeout: 30_000 });
    await expect(session.primaryStarInTab("Profile B")).toBeVisible({ timeout: 30_000 });
  });

  /**
   * Verifies that an on_turn_complete cascade that switches agent profiles
   * promotes the new session to primary.
   *
   * Note: Per PR #743 (pinnedSessionId), the user's pinned tab is preserved.
   * The active dockview tab stays on the pinned session; this test asserts on
   * the primary star marker.
   */
  test("on_turn_complete cascade promotes new agent session to primary", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const { profileA, profileB } = await createProfiles(apiClient);

    // Step1 (profileA, auto_start, move_to_next) → Step2 (profileB, auto_start)
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Cascade Tab Switch");
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

    const task = await apiClient.createTask(seedData.workspaceId, "Cascade Tab Task", {
      workflow_id: workflow.id,
      workflow_step_id: inbox.id,
      agent_profile_id: profileA.id,
      repository_ids: [seedData.repositoryId],
    });

    // Navigate to task page first so WS is connected
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await expect(session.chat).toBeVisible({ timeout: 15_000 });

    // Move to Step1 → auto_start with profileA → on_turn_complete → Step2 (profileB)
    await apiClient.moveTask(task.id, workflow.id, step1.id);

    // After the cascade, the Profile B tab should appear and own the primary star.
    const profileBTab = session.sessionTabByText("Profile B");
    await expect(profileBTab).toBeVisible({ timeout: 45_000 });
    await expect(session.primaryStarInTab("Profile B")).toBeVisible({ timeout: 45_000 });
  });

  /**
   * Verifies that after a manual step move, the new session tab shows
   * the primary star icon (★) without needing a page reload.
   * Covers the fix: switchSessionForStep uses SetPrimarySession (with WS
   * broadcast) instead of the raw repo.SetSessionPrimary.
   */
  test("primary star icon appears on new session tab after step move", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const { profileA, profileB } = await createProfiles(apiClient);

    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Primary Star Test");
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
      "Primary Star Task",
      profileA.id,
      {
        workflow_id: workflow.id,
        workflow_step_id: step1.id,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await expect(session.chat).toBeVisible({ timeout: 15_000 });

    // Wait for Profile A session to be ready
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.some((s) => s.state === "WAITING_FOR_INPUT");
        },
        { timeout: 30_000, message: "Waiting for agent to be ready" },
      )
      .toBe(true);

    // Move to Step2
    await apiClient.moveTask(task.id, workflow.id, step2.id);

    // The Profile B tab should appear and own the primary star (no reload needed).
    // Per PR #743, the active dockview tab stays pinned to Profile A — the star
    // moving is what proves SetPrimarySession's WS broadcast landed.
    const profileBTab = session.sessionTabByText("Profile B");
    await expect(profileBTab).toBeVisible({ timeout: 30_000 });
    await expect(session.primaryStarInTab("Profile B")).toBeVisible({ timeout: 30_000 });
  });

  /**
   * Verifies that moving to a step with NO agent_profile override reverts
   * to the task's original (default) agent profile from task metadata.
   * Covers the fix: maybySwitchSessionForProfile falls back to
   * task.Metadata["agent_profile_id"] when the step has no override.
   */
  test("moving to step without agent override reverts to task default agent", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const { profileA, profileB } = await createProfiles(apiClient);

    // Step1 (profileB, auto_start) → Step2 (NO override, auto_start)
    // Task created with profileA → Step1 overrides to profileB → Step2 should revert to profileA
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Revert Agent Test");
    const step1 = await apiClient.createWorkflowStep(workflow.id, "Step1", 0, {
      is_start_step: true,
    });
    const step2 = await apiClient.createWorkflowStep(workflow.id, "Step2", 1);
    await apiClient.createWorkflowStep(workflow.id, "Done", 2);

    await apiClient.updateWorkflowStep(step1.id, {
      agent_profile_id: profileB.id,
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });
    // Step2 has NO agent_profile_id — should revert to task default (profileA)
    await apiClient.updateWorkflowStep(step2.id, {
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Revert Agent Task",
      profileA.id,
      {
        workflow_id: workflow.id,
        workflow_step_id: step1.id,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await expect(session.chat).toBeVisible({ timeout: 15_000 });

    // Wait for Profile B session (Step1 override) to be ready
    const profileBTab = session.sessionTabByText("Profile B");
    await expect(profileBTab).toBeVisible({ timeout: 30_000 });
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.some((s) => s.state === "WAITING_FOR_INPUT");
        },
        { timeout: 30_000, message: "Waiting for agent to be ready" },
      )
      .toBe(true);

    // Move to Step2 (no override) — should revert to profileA
    await apiClient.moveTask(task.id, workflow.id, step2.id);

    // A Profile A session should appear and own the primary star (no reload needed).
    // Per PR #743, the user's pinned tab — Profile B from initial load — stays
    // active; the primary marker is what proves the revert-to-default landed.
    const profileATab = session.sessionTabByText("Profile A");
    await expect(profileATab).toBeVisible({ timeout: 30_000 });
    await expect(session.primaryStarInTab("Profile A")).toBeVisible({ timeout: 30_000 });
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
