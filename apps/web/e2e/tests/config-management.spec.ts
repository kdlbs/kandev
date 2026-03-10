import { test, expect } from "../fixtures/test-base";
import { SessionPage } from "../pages/session-page";
import type { ApiClient } from "../helpers/api-client";
import type { SeedData } from "../fixtures/test-base";

/**
 * Config management via agent MCP tools.
 *
 * These tests verify the config-mode MCP flow: when a task is created with
 * `config_mode: true` in its metadata, the agent session receives config-mode
 * MCP tools (list_workspaces, create_workflow_step, list_agents, etc.) that
 * let an AI agent configure Kandev via natural language.
 *
 * Config-mode is used by the dedicated "Config Chat" on the settings page.
 * Normal task sessions never receive config tools.
 *
 * The mock agent uses `e2e:mcp:kandev:<tool>(<json_args>)` script commands
 * to call real MCP tools through the agentctl MCP server.
 */

/** Creates a config-mode task via the API with config_mode metadata. */
function createConfigTask(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  prompt: string,
) {
  return apiClient.createTaskWithAgent(seedData.workspaceId, title, seedData.agentProfileId, {
    description: prompt,
    workflow_id: seedData.workflowId,
    repository_ids: [seedData.repositoryId],
    metadata: { config_mode: true, agent_profile_id: seedData.agentProfileId },
  });
}

test.describe("Config-mode MCP — workflow management", () => {
  test("agent can list workspaces via MCP tool", async ({ testPage, apiClient, seedData }) => {
    const task = await createConfigTask(
      apiClient,
      seedData,
      "List Workspaces Task",
      [
        'e2e:message("Listing workspaces...")',
        "e2e:mcp:kandev:list_workspaces({})",
        'e2e:message("Done listing workspaces")',
      ].join("\n"),
    );

    await testPage.goto(`/s/${task.session_id}`);

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.chat.getByText("Done listing workspaces", { exact: true })).toBeVisible({
      timeout: 30_000,
    });
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

    const task = await createConfigTask(
      apiClient,
      seedData,
      "Create Step Task",
      [
        'e2e:message("Creating QA Review step...")',
        `e2e:mcp:kandev:create_workflow_step({"workflow_id":"${workflow.id}","name":"QA Review","position":0})`,
        'e2e:message("QA Review step created")',
      ].join("\n"),
    );

    await testPage.goto(`/s/${task.session_id}`);

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.chat.getByText("QA Review step created", { exact: true })).toBeVisible({
      timeout: 30_000,
    });

    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    const qaStep = steps.find((s) => s.name === "QA Review");
    expect(qaStep).toBeTruthy();
  });
});

test.describe("Config-mode MCP — agent management", () => {
  test("agent can list agents via MCP tool", async ({ testPage, apiClient, seedData }) => {
    const task = await createConfigTask(
      apiClient,
      seedData,
      "List Agents Task",
      [
        'e2e:message("Listing agents...")',
        "e2e:mcp:kandev:list_agents({})",
        'e2e:message("Done listing agents")',
      ].join("\n"),
    );

    await testPage.goto(`/s/${task.session_id}`);

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.chat.getByText("Done listing agents", { exact: true })).toBeVisible({
      timeout: 30_000,
    });
    await expect(session.chat.getByText("list_agents", { exact: true })).toBeVisible({
      timeout: 10_000,
    });
  });

  test("agent can list agent profiles via MCP tool", async ({ testPage, apiClient, seedData }) => {
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];

    const task = await createConfigTask(
      apiClient,
      seedData,
      "List Profiles Task",
      [
        'e2e:message("Listing profiles...")',
        `e2e:mcp:kandev:list_agent_profiles({"agent_id":"${agent.id}"})`,
        'e2e:message("Done listing profiles")',
      ].join("\n"),
    );

    await testPage.goto(`/s/${task.session_id}`);

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.chat.getByText("Done listing profiles", { exact: true })).toBeVisible({
      timeout: 30_000,
    });
  });
});

test.describe("Config-mode MCP — multi-tool workflow", () => {
  test("agent executes multiple config MCP tools in sequence", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Multi-Tool Workflow");

    const task = await createConfigTask(
      apiClient,
      seedData,
      "Multi-Tool Task",
      [
        'e2e:message("Starting multi-tool config...")',
        "e2e:mcp:kandev:list_workspaces({})",
        `e2e:mcp:kandev:list_workflows({"workspace_id":"${seedData.workspaceId}"})`,
        `e2e:mcp:kandev:create_workflow_step({"workflow_id":"${workflow.id}","name":"Agent Created Step","position":0})`,
        'e2e:message("Multi-tool config complete")',
      ].join("\n"),
    );

    await testPage.goto(`/s/${task.session_id}`);

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.chat.getByText("Multi-tool config complete", { exact: true })).toBeVisible({
      timeout: 30_000,
    });

    // Multiple tool calls get collapsed into a turn group — expand it
    const toolCallsGroup = session.chat.getByRole("button", { name: /tool call/i });
    await expect(toolCallsGroup).toBeVisible({ timeout: 5_000 });
    await toolCallsGroup.click();

    await expect(session.chat.getByText("list_workspaces", { exact: true })).toBeVisible({
      timeout: 10_000,
    });
    await expect(session.chat.getByText("list_workflows", { exact: true })).toBeVisible({
      timeout: 10_000,
    });
    await expect(session.chat.getByText("create_workflow_step", { exact: true })).toBeVisible({
      timeout: 10_000,
    });

    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    const createdStep = steps.find((s) => s.name === "Agent Created Step");
    expect(createdStep).toBeTruthy();
  });
});
