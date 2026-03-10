import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";

/**
 * Config management via agent MCP tools.
 *
 * These tests verify the config-mode MCP flow: when a workflow step has
 * `enable_plan_mode` on_enter, the agent session receives config-mode MCP
 * tools (list_workspaces, create_workflow_step, list_agents, etc.) that
 * let an AI agent configure Kandev via natural language.
 *
 * The mock agent uses `e2e:mcp:kandev:<tool>(<json_args>)` script commands
 * to call real MCP tools through the agentctl MCP server.
 *
 * IMPORTANT: Step prompts must include `{{task_prompt}}` to avoid being
 * wrapped in <kandev-system> tags (which stripKandevSystem would discard).
 * The e2e: script commands must appear before {{task_prompt}} so they
 * are the first content after kandev-system blocks are stripped.
 */

/** Build a step prompt that puts e2e script commands before {{task_prompt}}. */
function scriptPrompt(...lines: string[]): string {
  return [...lines, "{{task_prompt}}"].join("\n");
}

test.describe("Config-mode MCP — workflow management", () => {
  test("agent can list workspaces via MCP tool", async ({ testPage, apiClient, seedData }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Config MCP Workflow");

    const inboxStep = await apiClient.createWorkflowStep(workflow.id, "Inbox", 0);
    const configStep = await apiClient.createWorkflowStep(workflow.id, "Configure", 1);

    // Configure step: auto_start + plan_mode → activates config-mode MCP tools.
    // The agent script calls list_workspaces via MCP.
    await apiClient.updateWorkflowStep(configStep.id, {
      prompt: scriptPrompt(
        'e2e:message("Listing workspaces...")',
        "e2e:mcp:kandev:list_workspaces({})",
        'e2e:message("Done listing workspaces")',
      ),
      events: {
        on_enter: [{ type: "auto_start_agent" }, { type: "enable_plan_mode" }],
      },
    });

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      enable_preview_on_click: false,
    });

    const task = await apiClient.createTask(seedData.workspaceId, "List Workspaces Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });

    // Move task to Configure step → triggers auto_start + plan_mode + config MCP
    await apiClient.moveTask(task.id, workflow.id, configStep.id);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardInColumn("List Workspaces Task", configStep.id);
    await expect(card).toBeVisible({ timeout: 30_000 });

    // Navigate to session
    await card.click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // The agent should have completed — check for the "Done" message
    await expect(session.chat.getByText("Done listing workspaces", { exact: true })).toBeVisible({
      timeout: 15_000,
    });

    // The MCP tool call should appear in the chat (tool_use block for list_workspaces)
    await expect(session.chat.getByText("list_workspaces", { exact: true })).toBeVisible({
      timeout: 10_000,
    });
  });

  test("agent can create a workflow step via MCP tool", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Step Creation Workflow");

    const inboxStep = await apiClient.createWorkflowStep(workflow.id, "Inbox", 0);
    const configStep = await apiClient.createWorkflowStep(workflow.id, "Configure", 1);

    // Agent will create a new workflow step named "QA Review" at position 2
    await apiClient.updateWorkflowStep(configStep.id, {
      prompt: scriptPrompt(
        'e2e:message("Creating QA Review step...")',
        `e2e:mcp:kandev:create_workflow_step({"workflow_id":"${workflow.id}","name":"QA Review","position":2})`,
        'e2e:message("QA Review step created")',
      ),
      events: {
        on_enter: [{ type: "auto_start_agent" }, { type: "enable_plan_mode" }],
      },
    });

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      enable_preview_on_click: false,
    });

    const task = await apiClient.createTask(seedData.workspaceId, "Create Step Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });

    await apiClient.moveTask(task.id, workflow.id, configStep.id);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardInColumn("Create Step Task", configStep.id);
    await expect(card).toBeVisible({ timeout: 30_000 });

    // Navigate to session to verify agent completed
    await card.click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.chat.getByText("QA Review step created", { exact: true })).toBeVisible({
      timeout: 15_000,
    });

    // Also check MCP tool result in chat (look for error indicators)
    const toolCallBlock = session.chat.getByText("create_workflow_step", { exact: true });
    await expect(toolCallBlock).toBeVisible({ timeout: 10_000 });

    // Verify the step was actually created via API
    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    const qaStep = steps.find((s) => s.name === "QA Review");
    expect(qaStep).toBeTruthy();
    expect(qaStep!.position).toBe(2);
  });
});

test.describe("Config-mode MCP — agent management", () => {
  test("agent can list agents via MCP tool", async ({ testPage, apiClient, seedData }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Agent List Workflow");

    const inboxStep = await apiClient.createWorkflowStep(workflow.id, "Inbox", 0);
    const configStep = await apiClient.createWorkflowStep(workflow.id, "Configure", 1);

    await apiClient.updateWorkflowStep(configStep.id, {
      prompt: scriptPrompt(
        'e2e:message("Listing agents...")',
        "e2e:mcp:kandev:list_agents({})",
        'e2e:message("Done listing agents")',
      ),
      events: {
        on_enter: [{ type: "auto_start_agent" }, { type: "enable_plan_mode" }],
      },
    });

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      enable_preview_on_click: false,
    });

    const task = await apiClient.createTask(seedData.workspaceId, "List Agents Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });

    await apiClient.moveTask(task.id, workflow.id, configStep.id);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardInColumn("List Agents Task", configStep.id);
    await expect(card).toBeVisible({ timeout: 30_000 });

    await card.click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Agent should have completed with both messages visible
    await expect(session.chat.getByText("Done listing agents", { exact: true })).toBeVisible({
      timeout: 15_000,
    });
    await expect(session.chat.getByText("list_agents", { exact: true })).toBeVisible({
      timeout: 10_000,
    });
  });

  test("agent can list agent profiles via MCP tool", async ({ testPage, apiClient, seedData }) => {
    // First get the agent ID
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];

    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Profile List Workflow");

    const inboxStep = await apiClient.createWorkflowStep(workflow.id, "Inbox", 0);
    const configStep = await apiClient.createWorkflowStep(workflow.id, "Configure", 1);

    await apiClient.updateWorkflowStep(configStep.id, {
      prompt: scriptPrompt(
        'e2e:message("Listing profiles...")',
        `e2e:mcp:kandev:list_agent_profiles({"agent_id":"${agent.id}"})`,
        'e2e:message("Done listing profiles")',
      ),
      events: {
        on_enter: [{ type: "auto_start_agent" }, { type: "enable_plan_mode" }],
      },
    });

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      enable_preview_on_click: false,
    });

    const task = await apiClient.createTask(seedData.workspaceId, "List Profiles Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });

    await apiClient.moveTask(task.id, workflow.id, configStep.id);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardInColumn("List Profiles Task", configStep.id);
    await expect(card).toBeVisible({ timeout: 30_000 });

    await card.click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.chat.getByText("Done listing profiles", { exact: true })).toBeVisible({
      timeout: 15_000,
    });
  });
});

test.describe("Config-mode MCP — task management", () => {
  test("agent can list tasks via MCP tool", async ({ testPage, apiClient, seedData }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Task List Workflow");

    const inboxStep = await apiClient.createWorkflowStep(workflow.id, "Inbox", 0);
    const configStep = await apiClient.createWorkflowStep(workflow.id, "Configure", 1);

    // Create a sample task so there's something to list
    await apiClient.createTask(seedData.workspaceId, "Sample Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
    });

    await apiClient.updateWorkflowStep(configStep.id, {
      prompt: scriptPrompt(
        'e2e:message("Listing tasks...")',
        `e2e:mcp:kandev:list_tasks({"workflow_id":"${workflow.id}"})`,
        'e2e:message("Done listing tasks")',
      ),
      events: {
        on_enter: [{ type: "auto_start_agent" }, { type: "enable_plan_mode" }],
      },
    });

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      enable_preview_on_click: false,
    });

    const configTask = await apiClient.createTask(seedData.workspaceId, "List Tasks Config Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });

    await apiClient.moveTask(configTask.id, workflow.id, configStep.id);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardInColumn("List Tasks Config Task", configStep.id);
    await expect(card).toBeVisible({ timeout: 30_000 });

    await card.click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.chat.getByText("Done listing tasks", { exact: true })).toBeVisible({
      timeout: 15_000,
    });
    await expect(session.chat.getByText("list_tasks", { exact: true })).toBeVisible({
      timeout: 10_000,
    });
  });

  test("agent can move a task between workflow steps via MCP tool", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Task Move Workflow");

    const inboxStep = await apiClient.createWorkflowStep(workflow.id, "Inbox", 0);
    const doneStep = await apiClient.createWorkflowStep(workflow.id, "Done", 1);
    const configStep = await apiClient.createWorkflowStep(workflow.id, "Configure", 2);

    // Create a target task to move
    const targetTask = await apiClient.createTask(seedData.workspaceId, "Target Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
    });

    // Agent will move the target task from Inbox to Done
    await apiClient.updateWorkflowStep(configStep.id, {
      prompt: scriptPrompt(
        'e2e:message("Moving task to Done...")',
        `e2e:mcp:kandev:move_task({"task_id":"${targetTask.id}","workflow_id":"${workflow.id}","workflow_step_id":"${doneStep.id}"})`,
        'e2e:message("Task moved successfully")',
      ),
      events: {
        on_enter: [{ type: "auto_start_agent" }, { type: "enable_plan_mode" }],
      },
    });

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      enable_preview_on_click: false,
    });

    const configTask = await apiClient.createTask(seedData.workspaceId, "Move Task Config Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });

    await apiClient.moveTask(configTask.id, workflow.id, configStep.id);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardInColumn("Move Task Config Task", configStep.id);
    await expect(card).toBeVisible({ timeout: 30_000 });

    await card.click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.chat.getByText("Task moved successfully", { exact: true })).toBeVisible({
      timeout: 15_000,
    });

    // Go back to kanban to verify the target task moved to Done column
    await kanban.goto();
    const targetInDone = kanban.taskCardInColumn("Target Task", doneStep.id);
    await expect(targetInDone).toBeVisible({ timeout: 10_000 });
  });
});

test.describe("Config-mode MCP — multi-tool workflow", () => {
  test("agent executes multiple config MCP tools in sequence", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Multi-Tool Workflow");

    const inboxStep = await apiClient.createWorkflowStep(workflow.id, "Inbox", 0);
    const configStep = await apiClient.createWorkflowStep(workflow.id, "Configure", 1);

    // Agent will: list workspaces, list workflows, create a step — all via MCP
    await apiClient.updateWorkflowStep(configStep.id, {
      prompt: scriptPrompt(
        'e2e:message("Starting multi-tool config...")',
        "e2e:mcp:kandev:list_workspaces({})",
        `e2e:mcp:kandev:list_workflows({"workspace_id":"${seedData.workspaceId}"})`,
        `e2e:mcp:kandev:create_workflow_step({"workflow_id":"${workflow.id}","name":"Agent Created Step","position":2})`,
        'e2e:message("Multi-tool config complete")',
      ),
      events: {
        on_enter: [{ type: "auto_start_agent" }, { type: "enable_plan_mode" }],
      },
    });

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      enable_preview_on_click: false,
    });

    const task = await apiClient.createTask(seedData.workspaceId, "Multi-Tool Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });

    await apiClient.moveTask(task.id, workflow.id, configStep.id);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardInColumn("Multi-Tool Task", configStep.id);
    await expect(card).toBeVisible({ timeout: 30_000 });

    await card.click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Wait for the agent to complete all tool calls
    await expect(session.chat.getByText("Multi-tool config complete", { exact: true })).toBeVisible(
      {
        timeout: 15_000,
      },
    );

    // Multiple tool calls get collapsed into a turn group — expand it
    const toolCallsGroup = session.chat.getByRole("button", { name: /tool call/i });
    await expect(toolCallsGroup).toBeVisible({ timeout: 5_000 });
    await toolCallsGroup.click();

    // Now individual MCP tool call names should be visible
    await expect(session.chat.getByText("list_workspaces", { exact: true })).toBeVisible({
      timeout: 10_000,
    });
    await expect(session.chat.getByText("list_workflows", { exact: true })).toBeVisible({
      timeout: 10_000,
    });
    await expect(session.chat.getByText("create_workflow_step", { exact: true })).toBeVisible({
      timeout: 10_000,
    });

    // Verify the step was actually created
    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    const createdStep = steps.find((s) => s.name === "Agent Created Step");
    expect(createdStep).toBeTruthy();
  });
});
