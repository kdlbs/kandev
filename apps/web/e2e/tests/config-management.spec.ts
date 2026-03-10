import { test, expect } from "../fixtures/test-base";
import { SessionPage } from "../pages/session-page";
import type { ApiClient } from "../helpers/api-client";
import type { SeedData } from "../fixtures/test-base";

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
function startConfigSession(
  apiClient: ApiClient,
  seedData: SeedData,
  prompt: string,
) {
  return apiClient.startConfigChat(seedData.workspaceId, seedData.agentProfileId, prompt);
}

// ---------------------------------------------------------------------------
// Workflow management
// ---------------------------------------------------------------------------

test.describe("Config-mode MCP — workflow management", () => {
  test("agent can list workspaces and workflows", async ({ testPage, apiClient, seedData }) => {
    const session = await startConfigSession(apiClient, seedData, [
      'e2e:message("Listing workspaces...")',
      "e2e:mcp:kandev:list_workspaces({})",
      `e2e:mcp:kandev:list_workflows({"workspace_id":"${seedData.workspaceId}"})`,
      'e2e:message("Done listing")',
    ].join("\n"));

    await testPage.goto(`/s/${session.session_id}`);
    const page = new SessionPage(testPage);
    await page.waitForLoad();

    await expect(page.chat.getByText("Done listing", { exact: true })).toBeVisible({
      timeout: 30_000,
    });
    await expect(page.chat.getByText("list_workspaces")).toBeVisible({ timeout: 10_000 });
    await expect(page.chat.getByText("list_workflows")).toBeVisible({ timeout: 10_000 });
  });

  test("agent can create and list workflow steps", async ({ testPage, apiClient, seedData }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Step CRUD Workflow");

    const session = await startConfigSession(apiClient, seedData, [
      'e2e:message("Creating step...")',
      `e2e:mcp:kandev:create_workflow_step({"workflow_id":"${workflow.id}","name":"QA Review","position":0})`,
      `e2e:mcp:kandev:list_workflow_steps({"workflow_id":"${workflow.id}"})`,
      'e2e:message("Steps listed")',
    ].join("\n"));

    await testPage.goto(`/s/${session.session_id}`);
    const page = new SessionPage(testPage);
    await page.waitForLoad();

    await expect(page.chat.getByText("Steps listed", { exact: true })).toBeVisible({
      timeout: 30_000,
    });

    // Verify via API
    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    const qaStep = steps.find((s) => s.name === "QA Review");
    expect(qaStep).toBeTruthy();
  });

  test("agent can update a workflow step", async ({ testPage, apiClient, seedData }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Update Step Workflow");
    const step = await apiClient.createWorkflowStep(workflow.id, "Draft", 0);

    const session = await startConfigSession(apiClient, seedData, [
      'e2e:message("Updating step...")',
      `e2e:mcp:kandev:update_workflow_step({"step_id":"${step.id}","name":"In Review","color":"#3b82f6"})`,
      'e2e:message("Step updated")',
    ].join("\n"));

    await testPage.goto(`/s/${session.session_id}`);
    const page = new SessionPage(testPage);
    await page.waitForLoad();

    await expect(page.chat.getByText("Step updated", { exact: true })).toBeVisible({
      timeout: 30_000,
    });

    // Verify via API
    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    const updated = steps.find((s) => s.id === step.id);
    expect(updated?.name).toBe("In Review");
  });
});

// ---------------------------------------------------------------------------
// Agent management
// ---------------------------------------------------------------------------

test.describe("Config-mode MCP — agent management", () => {
  test("agent can list agents and profiles", async ({ testPage, apiClient, seedData }) => {
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];

    const session = await startConfigSession(apiClient, seedData, [
      'e2e:message("Listing agents...")',
      "e2e:mcp:kandev:list_agents({})",
      `e2e:mcp:kandev:list_agent_profiles({"agent_id":"${agent.id}"})`,
      'e2e:message("Agents listed")',
    ].join("\n"));

    await testPage.goto(`/s/${session.session_id}`);
    const page = new SessionPage(testPage);
    await page.waitForLoad();

    await expect(page.chat.getByText("Agents listed", { exact: true })).toBeVisible({
      timeout: 30_000,
    });
    await expect(page.chat.getByText("list_agents")).toBeVisible({ timeout: 10_000 });
    await expect(page.chat.getByText("list_agent_profiles")).toBeVisible({ timeout: 10_000 });
  });

  test("agent can update an agent", async ({ testPage, apiClient, seedData }) => {
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];

    const session = await startConfigSession(apiClient, seedData, [
      'e2e:message("Updating agent...")',
      `e2e:mcp:kandev:update_agent({"agent_id":"${agent.id}","workspace_id":"${seedData.workspaceId}"})`,
      'e2e:message("Agent updated")',
    ].join("\n"));

    await testPage.goto(`/s/${session.session_id}`);
    const page = new SessionPage(testPage);
    await page.waitForLoad();

    await expect(page.chat.getByText("Agent updated", { exact: true })).toBeVisible({
      timeout: 30_000,
    });
  });

  test("agent can update an agent profile", async ({ testPage, apiClient, seedData }) => {
    const session = await startConfigSession(apiClient, seedData, [
      'e2e:message("Updating profile...")',
      `e2e:mcp:kandev:update_agent_profile({"profile_id":"${seedData.agentProfileId}","name":"Renamed Profile"})`,
      'e2e:message("Profile updated")',
    ].join("\n"));

    await testPage.goto(`/s/${session.session_id}`);
    const page = new SessionPage(testPage);
    await page.waitForLoad();

    await expect(page.chat.getByText("Profile updated", { exact: true })).toBeVisible({
      timeout: 30_000,
    });
  });
});

// ---------------------------------------------------------------------------
// MCP server configuration
// ---------------------------------------------------------------------------

test.describe("Config-mode MCP — MCP server configuration", () => {
  test("agent can get and update MCP config", async ({ testPage, apiClient, seedData }) => {
    const session = await startConfigSession(apiClient, seedData, [
      'e2e:message("Reading MCP config...")',
      `e2e:mcp:kandev:get_mcp_config({"profile_id":"${seedData.agentProfileId}"})`,
      `e2e:mcp:kandev:update_mcp_config({"profile_id":"${seedData.agentProfileId}","enabled":true,"servers":{"test-server":{"command":"node","args":["server.js"]}}})`,
      'e2e:message("MCP config updated")',
    ].join("\n"));

    await testPage.goto(`/s/${session.session_id}`);
    const page = new SessionPage(testPage);
    await page.waitForLoad();

    await expect(page.chat.getByText("MCP config updated", { exact: true })).toBeVisible({
      timeout: 30_000,
    });
    await expect(page.chat.getByText("get_mcp_config")).toBeVisible({ timeout: 10_000 });
    await expect(page.chat.getByText("update_mcp_config")).toBeVisible({ timeout: 10_000 });
  });
});

// ---------------------------------------------------------------------------
// Multi-tool workflow
// ---------------------------------------------------------------------------

test.describe("Config-mode MCP — multi-tool workflow", () => {
  test("agent executes multiple config tools in sequence", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Multi-Tool Workflow");

    const session = await startConfigSession(apiClient, seedData, [
      'e2e:message("Starting multi-tool config...")',
      "e2e:mcp:kandev:list_workspaces({})",
      `e2e:mcp:kandev:list_workflows({"workspace_id":"${seedData.workspaceId}"})`,
      `e2e:mcp:kandev:create_workflow_step({"workflow_id":"${workflow.id}","name":"Agent Created Step","position":0})`,
      "e2e:mcp:kandev:list_agents({})",
      'e2e:message("Multi-tool config complete")',
    ].join("\n"));

    await testPage.goto(`/s/${session.session_id}`);
    const page = new SessionPage(testPage);
    await page.waitForLoad();

    await expect(page.chat.getByText("Multi-tool config complete", { exact: true })).toBeVisible({
      timeout: 30_000,
    });

    // Verify the step was actually created
    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    const createdStep = steps.find((s) => s.name === "Agent Created Step");
    expect(createdStep).toBeTruthy();
  });
});
