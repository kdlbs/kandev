import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

/**
 * Config management via the dedicated config-chat endpoint.
 *
 * These tests verify the config-mode MCP flow: config chat sessions are
 * created via POST /workspaces/:id/config-chat, which sets config_mode: true
 * in session metadata. The agent then receives config-mode MCP tools
 * (workflow, agent, and MCP management) that are NOT available in regular
 * task sessions.
 *
 * The mock agent uses `e2e:mcp:kandev:<tool>(<json_args>)` script commands
 * to call real MCP tools through the agentctl MCP server.
 */

/** Creates a config chat session via the dedicated config-chat endpoint. */
function startConfigSession(apiClient: ApiClient, seedData: SeedData, prompt: string) {
  return apiClient.startConfigChat(seedData.workspaceId, seedData.agentProfileId, prompt);
}

/** Navigate to session page and wait for the final marker message. */
async function runAndWait(
  testPage: import("@playwright/test").Page,
  taskId: string,
  marker: string,
) {
  await testPage.goto(`/t/${taskId}`);
  const page = new SessionPage(testPage);
  await page.waitForLoad();
  await expect(page.chat.getByText(marker, { exact: true })).toBeVisible({ timeout: 30_000 });
  return page;
}

// ---------------------------------------------------------------------------
// Workflow management
// ---------------------------------------------------------------------------

test.describe("Config-mode MCP — workflow management", () => {
  test("agent can list workspaces and workflows", async ({ testPage, apiClient, seedData }) => {
    const session = await startConfigSession(
      apiClient,
      seedData,
      [
        'e2e:message("Listing workspaces...")',
        "e2e:mcp:kandev:list_workspaces({})",
        `e2e:mcp:kandev:list_workflows({"workspace_id":"${seedData.workspaceId}"})`,
        'e2e:message("Done listing")',
      ].join("\n"),
    );

    const page = await runAndWait(testPage, session.task_id, "Done listing");
    await expect(page.chat.getByText("list_workspaces")).toBeVisible({ timeout: 10_000 });
    await expect(page.chat.getByText("list_workflows")).toBeVisible({ timeout: 10_000 });
  });

  test("agent can create and list workflow steps", async ({ testPage, apiClient, seedData }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Step CRUD Workflow");

    const session = await startConfigSession(
      apiClient,
      seedData,
      [
        'e2e:message("Creating step...")',
        `e2e:mcp:kandev:create_workflow_step({"workflow_id":"${workflow.id}","name":"QA Review","position":0})`,
        `e2e:mcp:kandev:list_workflow_steps({"workflow_id":"${workflow.id}"})`,
        'e2e:message("Steps listed")',
      ].join("\n"),
    );

    await runAndWait(testPage, session.task_id, "Steps listed");

    // Verify via API
    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    const qaStep = steps.find((s) => s.name === "QA Review");
    expect(qaStep).toBeTruthy();
  });

  test("agent can create a step with all optional fields", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Full Fields Workflow");

    const createArgs = JSON.stringify({
      workflow_id: workflow.id,
      name: "Deploy",
      position: 0,
      color: "#22c55e",
      prompt: "Deploy to production",
      is_start_step: true,
      allow_manual_move: true,
      show_in_command_panel: true,
      events: {
        on_enter: [{ type: "auto_start_agent" }],
      },
    });

    const session = await startConfigSession(
      apiClient,
      seedData,
      [
        'e2e:message("Creating step with all fields...")',
        `e2e:mcp:kandev:create_workflow_step(${createArgs})`,
        'e2e:message("Step created")',
      ].join("\n"),
    );

    await runAndWait(testPage, session.task_id, "Step created");

    // Verify all fields via API
    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    const deploy = steps.find((s) => s.name === "Deploy");
    expect(deploy).toBeTruthy();
    expect(deploy!.color).toBe("#22c55e");
    expect(deploy!.prompt).toBe("Deploy to production");
    expect(deploy!.is_start_step).toBe(true);
    expect(deploy!.allow_manual_move).toBe(true);
    expect(deploy!.show_in_command_panel).toBe(true);
    expect(deploy!.events?.on_enter).toEqual([{ type: "auto_start_agent" }]);
  });

  test("agent can create a workflow", async ({ testPage, apiClient, seedData }) => {
    const session = await startConfigSession(
      apiClient,
      seedData,
      [
        'e2e:message("Creating workflow...")',
        `e2e:mcp:kandev:create_workflow({"workspace_id":"${seedData.workspaceId}","name":"E2E Workflow","description":"Created by E2E test"})`,
        'e2e:message("Workflow created")',
      ].join("\n"),
    );

    await runAndWait(testPage, session.task_id, "Workflow created");

    // Verify workflow was created via API
    const { workflows } = await apiClient.listWorkflows(seedData.workspaceId);
    const created = workflows.find((w) => w.name === "E2E Workflow");
    expect(created).toBeTruthy();
    expect(created!.description).toBe("Created by E2E test");
  });

  test("agent can update a workflow", async ({ testPage, apiClient, seedData }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Before Update");

    const session = await startConfigSession(
      apiClient,
      seedData,
      [
        'e2e:message("Updating workflow...")',
        `e2e:mcp:kandev:update_workflow({"workflow_id":"${workflow.id}","name":"After Update","description":"Updated description"})`,
        'e2e:message("Workflow updated")',
      ].join("\n"),
    );

    await runAndWait(testPage, session.task_id, "Workflow updated");

    // Verify via API
    const { workflows } = await apiClient.listWorkflows(seedData.workspaceId);
    const updated = workflows.find((w) => w.id === workflow.id);
    expect(updated?.name).toBe("After Update");
    expect(updated?.description).toBe("Updated description");
  });

  test("agent can delete a workflow", async ({ testPage, apiClient, seedData }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Deletable Workflow");
    // Create a bumper workflow so the config-chat session (which picks the newest
    // workflow via ORDER BY created_at DESC) doesn't land on the deletable one.
    // Otherwise ON DELETE CASCADE would wipe the config task/session mid-run.
    await apiClient.createWorkflow(seedData.workspaceId, "Config Anchor");

    const session = await startConfigSession(
      apiClient,
      seedData,
      [
        'e2e:message("Deleting workflow...")',
        `e2e:mcp:kandev:delete_workflow({"workflow_id":"${workflow.id}"})`,
        'e2e:message("Workflow deleted")',
      ].join("\n"),
    );

    await runAndWait(testPage, session.task_id, "Workflow deleted");

    // Verify workflow was deleted via API
    const { workflows } = await apiClient.listWorkflows(seedData.workspaceId);
    expect(workflows.find((w) => w.id === workflow.id)).toBeUndefined();
  });

  test("agent can update a workflow step", async ({ testPage, apiClient, seedData }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Update Step Workflow");
    const step = await apiClient.createWorkflowStep(workflow.id, "Draft", 0);

    const updateArgs = JSON.stringify({
      step_id: step.id,
      name: "In Review",
      color: "#3b82f6",
      allow_manual_move: true,
      show_in_command_panel: true,
      auto_archive_after_hours: 48,
      events: {
        on_enter: [{ type: "enable_plan_mode" }],
        on_turn_complete: [{ type: "move_to_next" }],
      },
    });

    const session = await startConfigSession(
      apiClient,
      seedData,
      [
        'e2e:message("Updating step...")',
        `e2e:mcp:kandev:update_workflow_step(${updateArgs})`,
        'e2e:message("Step updated")',
      ].join("\n"),
    );

    await runAndWait(testPage, session.task_id, "Step updated");

    // Verify via API
    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    const updated = steps.find((s) => s.id === step.id);
    expect(updated?.name).toBe("In Review");
    expect(updated?.color).toBe("#3b82f6");
    expect(updated?.allow_manual_move).toBe(true);
    expect(updated?.show_in_command_panel).toBe(true);
    expect(updated?.auto_archive_after_hours).toBe(48);
    expect(updated?.events?.on_enter).toEqual([{ type: "enable_plan_mode" }]);
    expect(updated?.events?.on_turn_complete).toEqual([{ type: "move_to_next" }]);
  });
});

// ---------------------------------------------------------------------------
// Agent management
// ---------------------------------------------------------------------------

test.describe("Config-mode MCP — agent management", () => {
  test("agent can list agents and profiles", async ({ testPage, apiClient, seedData }) => {
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];

    const session = await startConfigSession(
      apiClient,
      seedData,
      [
        'e2e:message("Listing agents...")',
        "e2e:mcp:kandev:list_agents({})",
        `e2e:mcp:kandev:list_agent_profiles({"agent_id":"${agent.id}"})`,
        'e2e:message("Agents listed")',
      ].join("\n"),
    );

    const page = await runAndWait(testPage, session.task_id, "Agents listed");
    await expect(page.chat.getByText("list_agents")).toBeVisible({ timeout: 10_000 });
    await expect(page.chat.getByText("list_agent_profiles")).toBeVisible({ timeout: 10_000 });
  });

  test("agent can create and delete an agent profile", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    const initialProfileCount = (agent.profiles ?? []).length;

    // Create a new profile via MCP tool
    const createSession = await startConfigSession(
      apiClient,
      seedData,
      [
        'e2e:message("Creating profile...")',
        `e2e:mcp:kandev:create_agent_profile({"agent_id":"${agent.id}","name":"E2E Created Profile","model":"claude-sonnet-4-5-20250514"})`,
        'e2e:message("Profile created")',
      ].join("\n"),
    );

    await runAndWait(testPage, createSession.task_id, "Profile created");

    // Verify profile was created via API
    const { agents: afterCreate } = await apiClient.listAgents();
    const agentAfterCreate = afterCreate.find((a) => a.id === agent.id);
    const newProfiles = (agentAfterCreate?.profiles ?? []).filter(
      (p) => p.name === "E2E Created Profile",
    );
    expect(newProfiles.length).toBe(1);
    expect(newProfiles[0].model).toBe("claude-sonnet-4-5-20250514");

    // Delete the profile via MCP tool
    const newProfileId = newProfiles[0].id;
    const deleteSession = await startConfigSession(
      apiClient,
      seedData,
      [
        'e2e:message("Deleting profile...")',
        `e2e:mcp:kandev:delete_agent_profile({"profile_id":"${newProfileId}"})`,
        'e2e:message("Profile deleted")',
      ].join("\n"),
    );

    await runAndWait(testPage, deleteSession.task_id, "Profile deleted");

    // Verify profile was deleted via API
    const { agents: afterDelete } = await apiClient.listAgents();
    const agentAfterDelete = afterDelete.find((a) => a.id === agent.id);
    expect((agentAfterDelete?.profiles ?? []).length).toBe(initialProfileCount);
    expect((agentAfterDelete?.profiles ?? []).find((p) => p.id === newProfileId)).toBeUndefined();
  });

  test("agent can update an agent", async ({ testPage, apiClient, seedData }) => {
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];

    const session = await startConfigSession(
      apiClient,
      seedData,
      [
        'e2e:message("Updating agent...")',
        `e2e:mcp:kandev:update_agent({"agent_id":"${agent.id}","supports_mcp":true})`,
        'e2e:message("Agent updated")',
      ].join("\n"),
    );

    await runAndWait(testPage, session.task_id, "Agent updated");

    // Verify via API
    const { agents: updated } = await apiClient.listAgents();
    const updatedAgent = updated.find((a) => a.id === agent.id);
    expect(updatedAgent?.supports_mcp).toBe(true);
  });

  test("agent can update an agent profile name", async ({ testPage, apiClient, seedData }) => {
    const session = await startConfigSession(
      apiClient,
      seedData,
      [
        'e2e:message("Updating profile...")',
        `e2e:mcp:kandev:update_agent_profile({"profile_id":"${seedData.agentProfileId}","name":"Renamed Profile"})`,
        'e2e:message("Profile updated")',
      ].join("\n"),
    );

    await runAndWait(testPage, session.task_id, "Profile updated");

    // Verify via API
    const { agents } = await apiClient.listAgents();
    const profile = agents
      .flatMap((a) => a.profiles ?? [])
      .find((p) => p.id === seedData.agentProfileId);
    expect(profile?.name).toBe("Renamed Profile");
  });

  test("agent can update agent profile model and auto_approve", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await startConfigSession(
      apiClient,
      seedData,
      [
        'e2e:message("Updating profile settings...")',
        `e2e:mcp:kandev:update_agent_profile({"profile_id":"${seedData.agentProfileId}","model":"claude-sonnet-4-5-20250514","auto_approve":false})`,
        'e2e:message("Profile settings updated")',
      ].join("\n"),
    );

    await runAndWait(testPage, session.task_id, "Profile settings updated");

    // Verify via API
    const { agents } = await apiClient.listAgents();
    const profile = agents
      .flatMap((a) => a.profiles ?? [])
      .find((p) => p.id === seedData.agentProfileId);
    expect(profile?.model).toBe("claude-sonnet-4-5-20250514");
    expect(profile?.auto_approve).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// MCP server configuration
// ---------------------------------------------------------------------------

test.describe("Config-mode MCP — MCP server configuration", () => {
  test("agent can get and update MCP config", async ({ testPage, apiClient, seedData }) => {
    const session = await startConfigSession(
      apiClient,
      seedData,
      [
        'e2e:message("Reading MCP config...")',
        `e2e:mcp:kandev:get_mcp_config({"profile_id":"${seedData.agentProfileId}"})`,
        `e2e:mcp:kandev:update_mcp_config({"profile_id":"${seedData.agentProfileId}","enabled":true,"servers":{"test-server":{"command":"node","args":["server.js"]}}})`,
        'e2e:message("MCP config updated")',
      ].join("\n"),
    );

    const page = await runAndWait(testPage, session.task_id, "MCP config updated");
    await expect(page.chat.getByText("get_mcp_config")).toBeVisible({ timeout: 10_000 });
    await expect(page.chat.getByText("update_mcp_config")).toBeVisible({ timeout: 10_000 });

    // Verify via API
    const config = await apiClient.getAgentProfileMcpConfig(seedData.agentProfileId);
    expect(config.enabled).toBe(true);
    expect(config.servers).toHaveProperty("test-server");
  });
});

// ---------------------------------------------------------------------------
// Task management
// ---------------------------------------------------------------------------

test.describe("Config-mode MCP — task management", () => {
  test("agent can list tasks in a workflow", async ({ testPage, apiClient, seedData }) => {
    // Create a task so there's something to list
    await apiClient.createTask(seedData.workspaceId, "Listable Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const session = await startConfigSession(
      apiClient,
      seedData,
      [
        'e2e:message("Listing tasks...")',
        `e2e:mcp:kandev:list_tasks({"workflow_id":"${seedData.workflowId}"})`,
        'e2e:message("Tasks listed")',
      ].join("\n"),
    );

    const page = await runAndWait(testPage, session.task_id, "Tasks listed");
    await expect(page.chat.getByText("list_tasks").first()).toBeVisible({ timeout: 10_000 });
  });

  test("agent can move a task to a different step", async ({ testPage, apiClient, seedData }) => {
    // Find a target step that is NOT the start step
    const targetStep = seedData.steps.find((s) => s.id !== seedData.startStepId);
    expect(targetStep).toBeTruthy();

    // Create a task in the start step
    const task = await apiClient.createTask(seedData.workspaceId, "Movable Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const session = await startConfigSession(
      apiClient,
      seedData,
      [
        'e2e:message("Moving task...")',
        `e2e:mcp:kandev:move_task({"task_id":"${task.id}","workflow_id":"${seedData.workflowId}","workflow_step_id":"${targetStep!.id}"})`,
        'e2e:message("Task moved")',
      ].join("\n"),
    );

    await runAndWait(testPage, session.task_id, "Task moved");

    // Verify the task is now in the target step
    const { tasks } = await apiClient.listTasks(seedData.workspaceId);
    const moved = tasks.find((t) => t.id === task.id);
    expect(moved?.workflow_step_id).toBe(targetStep!.id);
  });

  test("agent can archive a task", async ({ testPage, apiClient, seedData }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Archivable Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const session = await startConfigSession(
      apiClient,
      seedData,
      [
        'e2e:message("Archiving task...")',
        `e2e:mcp:kandev:archive_task({"task_id":"${task.id}"})`,
        'e2e:message("Task archived")',
      ].join("\n"),
    );

    await runAndWait(testPage, session.task_id, "Task archived");

    // Verify the task no longer appears in active task list
    const { tasks } = await apiClient.listTasks(seedData.workspaceId);
    expect(tasks.find((t) => t.id === task.id)).toBeUndefined();
  });

  test("agent can delete a task", async ({ testPage, apiClient, seedData }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Deletable Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const session = await startConfigSession(
      apiClient,
      seedData,
      [
        'e2e:message("Deleting task...")',
        `e2e:mcp:kandev:delete_task({"task_id":"${task.id}"})`,
        'e2e:message("Task deleted")',
      ].join("\n"),
    );

    await runAndWait(testPage, session.task_id, "Task deleted");

    // Verify the task is gone
    const { tasks } = await apiClient.listTasks(seedData.workspaceId);
    expect(tasks.find((t) => t.id === task.id)).toBeUndefined();
  });
});

// ---------------------------------------------------------------------------
// Executor management
// ---------------------------------------------------------------------------

test.describe("Config-mode MCP — executor management", () => {
  test("agent can list executors", async ({ testPage, apiClient, seedData }) => {
    const session = await startConfigSession(
      apiClient,
      seedData,
      [
        'e2e:message("Listing executors...")',
        "e2e:mcp:kandev:list_executors({})",
        'e2e:message("Executors listed")',
      ].join("\n"),
    );

    const page = await runAndWait(testPage, session.task_id, "Executors listed");
    await expect(page.chat.getByText("list_executors", { exact: true })).toBeVisible({
      timeout: 10_000,
    });
  });

  test("agent can create and delete an executor profile", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Use the system "Local" executor for the profile
    let { executors } = await apiClient.listExecutors();
    const executor = executors.find((e) => e.type === "local")!;

    // Create profile via MCP
    const createSession = await startConfigSession(
      apiClient,
      seedData,
      [
        'e2e:message("Creating executor profile...")',
        `e2e:mcp:kandev:create_executor_profile({"executor_id":"${executor.id}","name":"E2E Profile"})`,
        'e2e:message("Executor profile created")',
      ].join("\n"),
    );

    await runAndWait(testPage, createSession.task_id, "Executor profile created");

    // Verify via API
    ({ executors } = await apiClient.listExecutors());
    const exec = executors.find((e) => e.id === executor.id);
    const profile = exec?.profiles?.find((p) => p.name === "E2E Profile");
    expect(profile).toBeTruthy();

    // Delete profile via MCP
    const deleteSession = await startConfigSession(
      apiClient,
      seedData,
      [
        'e2e:message("Deleting executor profile...")',
        `e2e:mcp:kandev:delete_executor_profile({"profile_id":"${profile!.id}"})`,
        'e2e:message("Executor profile deleted")',
      ].join("\n"),
    );

    await runAndWait(testPage, deleteSession.task_id, "Executor profile deleted");

    // Verify deleted
    const { executors: afterDelete } = await apiClient.listExecutors();
    const execAfter = afterDelete.find((e) => e.id === executor.id);
    expect(execAfter?.profiles?.find((p) => p.id === profile!.id)).toBeUndefined();
  });
});

// ---------------------------------------------------------------------------
// Multi-tool workflows
// ---------------------------------------------------------------------------

test.describe("Config-mode MCP — multi-tool workflow", () => {
  test("agent executes multiple config tools in sequence", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Multi-Tool Workflow");

    const session = await startConfigSession(
      apiClient,
      seedData,
      [
        'e2e:message("Starting multi-tool config...")',
        "e2e:mcp:kandev:list_workspaces({})",
        `e2e:mcp:kandev:list_workflows({"workspace_id":"${seedData.workspaceId}"})`,
        `e2e:mcp:kandev:create_workflow_step({"workflow_id":"${workflow.id}","name":"Agent Created Step","position":0})`,
        "e2e:mcp:kandev:list_agents({})",
        'e2e:message("Multi-tool config complete")',
      ].join("\n"),
    );

    await runAndWait(testPage, session.task_id, "Multi-tool config complete");

    // Verify the step was actually created
    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    const createdStep = steps.find((s) => s.name === "Agent Created Step");
    expect(createdStep).toBeTruthy();
  });

  test("agent performs full workflow setup with steps, profile, and MCP config", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Full Setup Workflow");
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];

    const session = await startConfigSession(
      apiClient,
      seedData,
      [
        'e2e:message("Setting up full workflow...")',
        // Create 3 steps
        `e2e:mcp:kandev:create_workflow_step({"workflow_id":"${workflow.id}","name":"Build","position":0,"color":"#3b82f6"})`,
        `e2e:mcp:kandev:create_workflow_step({"workflow_id":"${workflow.id}","name":"Test","position":1,"color":"#eab308"})`,
        `e2e:mcp:kandev:create_workflow_step({"workflow_id":"${workflow.id}","name":"Deploy","position":2,"color":"#22c55e"})`,
        // Create a new agent profile
        `e2e:mcp:kandev:create_agent_profile({"agent_id":"${agent.id}","name":"CI Profile","model":"claude-sonnet-4-5-20250514"})`,
        // Update MCP config on the test profile
        `e2e:mcp:kandev:update_mcp_config({"profile_id":"${seedData.agentProfileId}","enabled":true,"servers":{"ci-tools":{"command":"npx","args":["-y","@ci/tools"]}}})`,
        'e2e:message("Full setup complete")',
      ].join("\n"),
    );

    await runAndWait(testPage, session.task_id, "Full setup complete");

    // Verify steps
    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    expect(steps.length).toBe(3);
    expect(steps.map((s) => s.name).sort()).toEqual(["Build", "Deploy", "Test"]);

    // Verify new profile was created
    const { agents: updatedAgents } = await apiClient.listAgents();
    const updatedAgent = updatedAgents.find((a) => a.id === agent.id);
    expect((updatedAgent?.profiles ?? []).find((p) => p.name === "CI Profile")).toBeTruthy();

    // Verify MCP config
    const config = await apiClient.getAgentProfileMcpConfig(seedData.agentProfileId);
    expect(config.enabled).toBe(true);
    expect(config.servers).toHaveProperty("ci-tools");
  });
});
