import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";

test.describe("Config management — workflow steps", () => {
  test("new workflow step appears as kanban column", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Create a new step via API
    const step = await apiClient.createWorkflowStep(
      seedData.workflowId,
      "QA Review",
      seedData.steps.length,
    );

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // The new step should render as a column
    const column = kanban.columnByStepId(step.id);
    await expect(column).toBeVisible({ timeout: 10_000 });
    await expect(column).toContainText("QA Review");
  });

  test("deleting a workflow step removes the kanban column", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Create a step, then delete it
    const step = await apiClient.createWorkflowStep(
      seedData.workflowId,
      "Ephemeral Step",
      seedData.steps.length,
    );
    await apiClient.deleteWorkflowStep(step.id);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const column = kanban.columnByStepId(step.id);
    await expect(column).not.toBeVisible();
  });

  test("task placed in a new step renders in correct column", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const step = await apiClient.createWorkflowStep(
      seedData.workflowId,
      "Deploy",
      seedData.steps.length,
    );

    const task = await apiClient.createTask(seedData.workspaceId, "Deploy Feature X", {
      workflow_id: seedData.workflowId,
      workflow_step_id: step.id,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardInColumn("Deploy Feature X", step.id);
    await expect(card).toBeVisible({ timeout: 10_000 });
    expect(task.id).toBeTruthy();
  });
});

test.describe("Config management — task operations", () => {
  test("task moves between columns via API", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const startStep = seedData.steps.find((s) => s.is_start_step) ?? seedData.steps[0];
    const targetStep = seedData.steps.find((s) => s.id !== startStep.id);
    if (!targetStep) {
      test.skip(true, "Need at least 2 workflow steps");
      return;
    }

    const task = await apiClient.createTask(seedData.workspaceId, "Move Me", {
      workflow_id: seedData.workflowId,
      workflow_step_id: startStep.id,
    });

    // Move the task to the target step
    await apiClient.moveTask(task.id, seedData.workflowId, targetStep.id);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // Should be in the target column
    const cardInTarget = kanban.taskCardInColumn("Move Me", targetStep.id);
    await expect(cardInTarget).toBeVisible({ timeout: 10_000 });

    // Should NOT be in the start column
    const cardInStart = kanban.taskCardInColumn("Move Me", startStep.id);
    await expect(cardInStart).not.toBeVisible();
  });

  test("archived task disappears from kanban", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Archivable Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCardByTitle("Archivable Task")).toBeVisible({ timeout: 10_000 });

    // Archive via API
    await apiClient.archiveTask(task.id);

    // Task card should disappear (WS push removes it from board)
    await expect(kanban.taskCardByTitle("Archivable Task")).not.toBeVisible({ timeout: 15_000 });
  });

  test("deleted task disappears from kanban", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Deletable Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCard(task.id)).toBeVisible({ timeout: 10_000 });

    // Delete via API
    await apiClient.deleteTask(task.id);

    // Task card should disappear
    await expect(kanban.taskCard(task.id)).not.toBeVisible({ timeout: 15_000 });
  });
});

test.describe("Config management — agents & profiles", () => {
  test("agent settings page lists available agents", async ({
    testPage,
  }) => {
    await testPage.goto("/settings/agents");

    // The supported agents section and Mock agent card should appear
    await expect(testPage.getByText("Supported agents found")).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByText("Mock").first()).toBeVisible();
  });

  test("agent profile page shows profile details", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Use the seed profile which persists across test resets
    const { agents } = await apiClient.listAgents();
    const agent = agents.find((a) => a.profiles?.some((p) => p.id === seedData.agentProfileId));
    if (!agent) {
      test.skip(true, "No agent with seed profile found");
      return;
    }

    await testPage.goto(`/settings/agents/${encodeURIComponent(agent.name)}/profiles/${seedData.agentProfileId}`);

    // Profile page should show profile settings section
    await expect(testPage.getByText("Profile settings", { exact: true })).toBeVisible({ timeout: 15_000 });
  });

  test("creating and deleting an agent profile via API is reflected in settings", async ({
    testPage,
    apiClient,
  }) => {
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];

    // Create a new profile
    const profile = await apiClient.createAgentProfile(agent.id, "E2E Test Profile", {
      model: agent.profiles[0]?.model ?? "default",
      auto_approve: true,
    });

    // Navigate to agent page — new profile should be visible
    await testPage.goto(`/settings/agents/${encodeURIComponent(agent.name)}`);
    await expect(testPage.getByText("E2E Test Profile")).toBeVisible({ timeout: 15_000 });

    // Delete and reload — profile should be gone
    await apiClient.deleteAgentProfile(profile.id);
    await testPage.reload();
    await expect(testPage.getByText("E2E Test Profile")).not.toBeVisible({ timeout: 10_000 });
  });

  test("MCP config section is visible on profile page", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Use the seed profile which persists across test resets
    const { agents } = await apiClient.listAgents();
    const agent = agents.find((a) => a.supports_mcp && a.profiles?.some((p) => p.id === seedData.agentProfileId));
    if (!agent) {
      test.skip(true, "No MCP-capable agent with seed profile available");
      return;
    }

    await testPage.goto(`/settings/agents/${encodeURIComponent(agent.name)}/profiles/${seedData.agentProfileId}`);

    // The MCP Configuration card should be visible
    await expect(testPage.getByText("MCP Configuration")).toBeVisible({ timeout: 15_000 });
  });
});

test.describe("Config management — workflow settings", () => {
  test("workflow settings page loads and shows pipeline editor", async ({
    testPage,
    seedData,
  }) => {
    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/workflows`);

    // The workflow pipeline editor should be visible with steps
    await expect(testPage.getByText("Workflow Steps")).toBeVisible({ timeout: 15_000 });
    // The workflow name input should contain the seeded workflow name
    await expect(testPage.locator('input[value="E2E Workflow"]')).toBeVisible();
  });

  test("workflow step events persist through API round-trip", async ({
    apiClient,
    seedData,
  }) => {
    // Create a step with automation events
    const step = await apiClient.createWorkflowStep(
      seedData.workflowId,
      "Automated Step",
      seedData.steps.length,
    );

    await apiClient.updateWorkflowStep(step.id, {
      prompt: "Run the automated tests",
      events: {
        on_enter: [{ type: "auto_start_agent" }],
        on_turn_complete: [{ type: "move_to_next" }],
      },
    });

    // Fetch steps and verify events persisted
    const { steps } = await apiClient.listWorkflowSteps(seedData.workflowId);
    const updated = steps.find((s) => s.id === step.id);
    expect(updated).toBeTruthy();
    expect(updated!.prompt).toBe("Run the automated tests");
    expect(updated!.events?.on_enter).toEqual(
      expect.arrayContaining([expect.objectContaining({ type: "auto_start_agent" })]),
    );
    expect(updated!.events?.on_turn_complete).toEqual(
      expect.arrayContaining([expect.objectContaining({ type: "move_to_next" })]),
    );
  });

  test("MCP config round-trip via API", async ({ apiClient, seedData }) => {
    // Get the agent profile's MCP config
    const config = await apiClient.getAgentProfileMcpConfig(seedData.agentProfileId);
    expect(config.profile_id).toBe(seedData.agentProfileId);
    // Default state: should have enabled field
    expect(typeof config.enabled).toBe("boolean");
  });
});
