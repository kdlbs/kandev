import { test, expect, type SeedData } from "../../fixtures/test-base";
import { waitForAgentMessage, waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { Page } from "@playwright/test";
import { getMockAgent } from "../../helpers/agent-fixtures";

/** Each journey retains the real config-mode agentctl MCP transport and API persistence checks. */
type ConfigCall = [name: string, args: Record<string, unknown>];

function configSessionRunner(apiClient: ApiClient, seedData: SeedData, testPage: Page) {
  let session: { task_id: string; session_id: string } | undefined;
  return async (calls: ConfigCall[], marker: string) => {
    const prompt = [
      ...calls.map(([name, args]) => `e2e:mcp:kandev:${name}(${JSON.stringify(args)})`),
      `e2e:message(${JSON.stringify(marker)})`,
    ].join("\n");
    if (!session) {
      session = await apiClient.startConfigChat(
        seedData.workspaceId,
        seedData.agentProfileId,
        prompt,
      );
    } else {
      await apiClient.addUserMessage(session.task_id, session.session_id, prompt);
    }
    return runAndWait(testPage, apiClient, session.task_id, marker);
  };
}

async function runAndWait(testPage: Page, apiClient: ApiClient, taskId: string, marker: string) {
  const { sessions } = await apiClient.listTaskSessions(taskId);
  expect(sessions).toHaveLength(1);
  const sessionId = sessions[0].id;
  await waitForAgentMessage(apiClient, sessionId, marker, 30_000);
  await waitForSessionDone(
    apiClient,
    taskId,
    sessionId,
    `config turn containing ${marker} should complete`,
    30_000,
  );
  if (new URL(testPage.url()).pathname !== `/t/${taskId}`) {
    await testPage.goto(`/t/${taskId}`);
  }
  const page = new SessionPage(testPage);
  await expect(page.activeChat()).toBeVisible({ timeout: 15_000 });
  await expect(page.anyIdleInput()).toBeVisible({ timeout: 15_000 });
  const markerMessage = page
    .activeChat()
    .getByTestId("agent-message-highlight")
    .filter({ hasText: marker })
    .last();
  await expect(markerMessage).toContainText(marker);
  return page;
}

async function expandToolCallGroups(page: SessionPage) {
  const headers = page.chat.getByRole("button", { name: /\d+ tool calls?$/ });
  for (let i = 0; i < (await headers.count()); i++) {
    if ((await headers.nth(i).getAttribute("aria-expanded")) !== "true")
      await headers.nth(i).click();
  }
}

test("workflow CRUD and cross-resource setup preserve every config tool result", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  const runConfig = configSessionRunner(apiClient, seedData, testPage);
  const originalMcp = await apiClient.getAgentProfileMcpConfig(seedData.agentProfileId);
  try {
    await test.step("agent can list workspaces and workflows", async () => {
      const page = await runConfig(
        [
          ["list_workspaces_kandev", {}],
          ["list_workflows_kandev", { workspace_id: seedData.workspaceId }],
        ],
        "Done listing",
      );
      await expandToolCallGroups(page);
      await expect(page.chat.getByText("Kandev: List Workspaces")).toBeVisible({ timeout: 10_000 });
      await expect(page.chat.getByText("Kandev: List Workflows")).toBeVisible({ timeout: 10_000 });
    });

    // The config session already exists before any deletable workflow is created.
    const minimal = await apiClient.createWorkflow(seedData.workspaceId, "Step CRUD Workflow");
    const fullFields = await apiClient.createWorkflow(seedData.workspaceId, "Full Fields Workflow");
    const renamed = await apiClient.createWorkflow(seedData.workspaceId, "Before Update");
    const deleted = await apiClient.createWorkflow(seedData.workspaceId, "Deletable Workflow");
    const stepWorkflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Update Step Workflow",
    );
    const step = await apiClient.createWorkflowStep(stepWorkflow.id, "Draft", 0);
    const multiTool = await apiClient.createWorkflow(seedData.workspaceId, "Multi-Tool Workflow");
    const fullSetup = await apiClient.createWorkflow(seedData.workspaceId, "Full Setup Workflow");
    const { agents } = await apiClient.listAgents();
    const agent = getMockAgent(agents);

    await runConfig(
      [
        [
          "create_workflow_step_kandev",
          { workflow_id: minimal.id, name: "QA Review", position: 0 },
        ],
        ["list_workflow_steps_kandev", { workflow_id: minimal.id }],
        [
          "create_workflow_step_kandev",
          {
            workflow_id: fullFields.id,
            name: "Deploy",
            position: 0,
            color: "#22c55e",
            prompt: "Deploy to production",
            is_start_step: true,
            allow_manual_move: true,
            show_in_command_panel: true,
            events: { on_enter: [{ type: "auto_start_agent" }] },
          },
        ],
        [
          "create_workflow_kandev",
          {
            workspace_id: seedData.workspaceId,
            name: "E2E Workflow",
            description: "Created by E2E test",
          },
        ],
        [
          "update_workflow_kandev",
          {
            workflow_id: renamed.id,
            name: "After Update",
            description: "Updated description",
          },
        ],
        ["delete_workflow_kandev", { workflow_id: deleted.id }],
        [
          "update_workflow_step_kandev",
          {
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
          },
        ],
        ["list_workspaces_kandev", {}],
        ["list_workflows_kandev", { workspace_id: seedData.workspaceId }],
        [
          "create_workflow_step_kandev",
          { workflow_id: multiTool.id, name: "Agent Created Step", position: 0 },
        ],
        ["list_agents_kandev", {}],
        [
          "create_workflow_step_kandev",
          { workflow_id: fullSetup.id, name: "Build", position: 0, color: "#3b82f6" },
        ],
        [
          "create_agent_profile_kandev",
          { agent_id: agent.id, name: "CI Profile", model: "mock-fast" },
        ],
        [
          "update_mcp_config_kandev",
          {
            profile_id: seedData.agentProfileId,
            enabled: true,
            servers: { "ci-tools": { command: "npx", args: ["-y", "@ci/tools"] } },
          },
        ],
      ],
      "Workflow operations complete",
    );

    await test.step("agent can create and list workflow steps", async () => {
      const { steps } = await apiClient.listWorkflowSteps(minimal.id);
      expect(steps.find((s) => s.name === "QA Review")).toBeTruthy();
    });
    await test.step("agent can create a step with all optional fields", async () => {
      const { steps } = await apiClient.listWorkflowSteps(fullFields.id);
      const deploy = steps.find((s) => s.name === "Deploy");
      expect(deploy).toBeTruthy();
      expect(deploy!.color).toBe("#22c55e");
      expect(deploy!.prompt).toBe("Deploy to production");
      expect(deploy!.is_start_step).toBe(true);
      expect(deploy!.allow_manual_move).toBe(true);
      expect(deploy!.show_in_command_panel).toBe(true);
      expect(deploy!.events?.on_enter).toEqual([{ type: "auto_start_agent" }]);
    });
    await test.step("agent can create a workflow", async () => {
      const { workflows } = await apiClient.listWorkflows(seedData.workspaceId);
      expect(
        workflows.find((w) => w.name === "E2E Workflow" && w.description === "Created by E2E test"),
      ).toBeTruthy();
    });
    await test.step("agent can update a workflow", async () => {
      const { workflows } = await apiClient.listWorkflows(seedData.workspaceId);
      const updated = workflows.find((w) => w.id === renamed.id);
      expect(updated?.name).toBe("After Update");
      expect(updated?.description).toBe("Updated description");
    });
    await test.step("agent can delete a workflow", async () => {
      const { workflows } = await apiClient.listWorkflows(seedData.workspaceId);
      expect(workflows.find((w) => w.id === deleted.id)).toBeUndefined();
    });
    await test.step("agent can update a workflow step", async () => {
      const { steps } = await apiClient.listWorkflowSteps(stepWorkflow.id);
      const updated = steps.find((s) => s.id === step.id);
      expect(updated?.name).toBe("In Review");
      expect(updated?.color).toBe("#3b82f6");
      expect(updated?.allow_manual_move).toBe(true);
      expect(updated?.show_in_command_panel).toBe(true);
      expect(updated?.auto_archive_after_hours).toBe(48);
      expect(updated?.events?.on_enter).toEqual([{ type: "enable_plan_mode" }]);
      expect(updated?.events?.on_turn_complete).toEqual([{ type: "move_to_next" }]);
    });
    await test.step("agent executes multiple config tools in sequence", async () => {
      const { steps } = await apiClient.listWorkflowSteps(multiTool.id);
      expect(steps.find((s) => s.name === "Agent Created Step")).toBeTruthy();
    });
    await test.step("agent performs workflow setup with a step, profile, and MCP config", async () => {
      const { steps } = await apiClient.listWorkflowSteps(fullSetup.id);
      expect(steps).toHaveLength(1);
      expect(steps).toEqual(
        expect.arrayContaining([
          expect.objectContaining({ name: "Build", position: 0, color: "#3b82f6" }),
        ]),
      );
      const { agents: updatedAgents } = await apiClient.listAgents();
      const updatedAgent = updatedAgents.find((a) => a.id === agent.id);
      expect((updatedAgent?.profiles ?? []).find((p) => p.name === "CI Profile")).toBeTruthy();
      const config = await apiClient.getAgentProfileMcpConfig(seedData.agentProfileId);
      expect(config.enabled).toBe(true);
      expect(config.servers).toHaveProperty("ci-tools");
    });
  } finally {
    await apiClient.updateAgentProfileMcpConfig(seedData.agentProfileId, originalMcp);
  }
});

test("agent and profile lifecycle preserves model, name and MCP configuration", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  const runConfig = configSessionRunner(apiClient, seedData, testPage);
  const originalProfile = await apiClient.getAgentProfile(seedData.agentProfileId);
  const originalMcp = await apiClient.getAgentProfileMcpConfig(seedData.agentProfileId);
  const { agents } = await apiClient.listAgents();
  const firstAgent = agents[0];
  const mockAgent = getMockAgent(agents);
  const initialProfileCount = (mockAgent.profiles ?? []).length;
  try {
    const page = await runConfig(
      [
        ["list_agents_kandev", {}],
        ["list_agent_profiles_kandev", { agent_id: firstAgent.id }],
        [
          "create_agent_profile_kandev",
          { agent_id: mockAgent.id, name: "E2E Created Profile", model: "mock-fast" },
        ],
        ["update_agent_kandev", { agent_id: firstAgent.id, supports_mcp: true }],
        [
          "update_agent_profile_kandev",
          { profile_id: seedData.agentProfileId, name: "Renamed Profile" },
        ],
        [
          "update_agent_profile_kandev",
          { profile_id: seedData.agentProfileId, model: "claude-sonnet-4-5-20250514" },
        ],
        ["get_mcp_config_kandev", { profile_id: seedData.agentProfileId }],
        [
          "update_mcp_config_kandev",
          {
            profile_id: seedData.agentProfileId,
            enabled: true,
            servers: { "test-server": { command: "node", args: ["server.js"] } },
          },
        ],
      ],
      "Profile operations complete",
    );
    await expandToolCallGroups(page);
    await test.step("agent can list agents and profiles", async () => {
      await expect(page.chat.getByText("Kandev: List Agents")).toBeVisible({ timeout: 10_000 });
      await expect(page.chat.getByText("Kandev: List Agent Profiles")).toBeVisible({
        timeout: 10_000,
      });
    });
    await test.step("agent can update an agent", async () => {
      const { agents: updated } = await apiClient.listAgents();
      expect(updated.find((a) => a.id === firstAgent.id)?.supports_mcp).toBe(true);
    });
    await test.step("agent can update an agent profile name", async () => {
      const { agents: updated } = await apiClient.listAgents();
      const profile = updated
        .flatMap((a) => a.profiles ?? [])
        .find((p) => p.id === seedData.agentProfileId);
      expect(profile?.name).toBe("Renamed Profile");
    });
    await test.step("agent can update agent profile model via MCP", async () => {
      const profile = await apiClient.getAgentProfile(seedData.agentProfileId);
      expect(profile.model).toBe("claude-sonnet-4-5-20250514");
    });
    await test.step("agent can get and update MCP config", async () => {
      await expect(page.chat.getByText("Kandev: Get MCP Config")).toBeVisible({ timeout: 10_000 });
      await expect(page.chat.getByText("Kandev: Update MCP Config")).toBeVisible({
        timeout: 10_000,
      });
      const config = await apiClient.getAgentProfileMcpConfig(seedData.agentProfileId);
      expect(config.enabled).toBe(true);
      expect(config.servers).toHaveProperty("test-server");
    });
    // Restore runtime inputs before the next prompt starts another real turn.
    await apiClient.updateAgentProfile(seedData.agentProfileId, { model: originalProfile.model });
    await apiClient.updateAgentProfileMcpConfig(seedData.agentProfileId, originalMcp);
    await test.step("agent can create and delete an agent profile", async () => {
      const getProfiles = async () => {
        const { agents: current } = await apiClient.listAgents();
        return current.find((a) => a.id === mockAgent.id)?.profiles ?? [];
      };
      const getCreatedProfiles = async () =>
        (await getProfiles()).filter((p) => p.name === "E2E Created Profile");
      await expect
        .poll(async () => (await getCreatedProfiles()).length, { timeout: 10_000 })
        .toBe(1);
      const newProfiles = await getCreatedProfiles();
      expect(newProfiles.length).toBe(1);
      expect(newProfiles[0].model).toBe("mock-fast");
      const newProfileId = newProfiles[0].id;
      await runConfig(
        [["delete_agent_profile_kandev", { profile_id: newProfileId }]],
        "Profile deleted",
      );
      await expect
        .poll(async () => (await getProfiles()).some((p) => p.id === newProfileId), {
          timeout: 10_000,
        })
        .toBe(false);
      expect(await getProfiles()).toHaveLength(initialProfileCount);
    });
  } finally {
    await apiClient.updateAgentProfile(seedData.agentProfileId, {
      name: originalProfile.name,
      model: originalProfile.model,
    });
    await apiClient.updateAgentProfileMcpConfig(seedData.agentProfileId, originalMcp);
  }
});

test("task listing, movement, archive and deletion use config-mode MCP", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const runConfig = configSessionRunner(apiClient, seedData, testPage);
  const targetStep = seedData.steps.find((s) => s.id !== seedData.startStepId);
  expect(targetStep).toBeTruthy();
  const taskOptions = { workflow_id: seedData.workflowId, workflow_step_id: seedData.startStepId };
  await apiClient.createTask(seedData.workspaceId, "Listable Task", taskOptions);
  const moved = await apiClient.createTask(seedData.workspaceId, "Movable Task", taskOptions);
  const archived = await apiClient.createTask(seedData.workspaceId, "Archivable Task", taskOptions);
  const deleted = await apiClient.createTask(seedData.workspaceId, "Deletable Task", taskOptions);
  const page = await runConfig(
    [
      ["list_tasks_kandev", { workflow_id: seedData.workflowId }],
      [
        "move_task_kandev",
        { task_id: moved.id, workflow_id: seedData.workflowId, workflow_step_id: targetStep!.id },
      ],
      ["archive_task_kandev", { task_id: archived.id }],
      ["delete_task_kandev", { task_id: deleted.id }],
    ],
    "Task operations complete",
  );
  await test.step("agent can list tasks in a workflow", async () => {
    await expandToolCallGroups(page);
    await expect(page.chat.getByText("Kandev: List Tasks").first()).toBeVisible({
      timeout: 10_000,
    });
  });
  await test.step("agent can move a task to a different step", async () => {
    const { tasks } = await apiClient.listTasks(seedData.workspaceId);
    expect(tasks.find((t) => t.id === moved.id)?.workflow_step_id).toBe(targetStep!.id);
  });
  await test.step("agent can archive a task", async () => {
    const { tasks } = await apiClient.listTasks(seedData.workspaceId);
    expect(tasks.find((t) => t.id === archived.id)).toBeUndefined();
  });
  await test.step("agent can delete a task", async () => {
    const { tasks } = await apiClient.listTasks(seedData.workspaceId);
    expect(tasks.find((t) => t.id === deleted.id)).toBeUndefined();
  });
});

test("executor listing and profile lifecycle use config-mode MCP", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const runConfig = configSessionRunner(apiClient, seedData, testPage);
  const { executors } = await apiClient.listExecutors();
  const executor = executors.find((e) => e.type === "local")!;
  const page = await runConfig(
    [
      ["list_executors_kandev", {}],
      ["create_executor_profile_kandev", { executor_id: executor.id, name: "E2E Profile" }],
    ],
    "Executor profile created",
  );
  await test.step("agent can list executors", async () => {
    await expandToolCallGroups(page);
    await expect(page.chat.getByText("Kandev: List Executors", { exact: true })).toBeVisible({
      timeout: 10_000,
    });
  });
  await test.step("agent can create and delete an executor profile", async () => {
    const { executors: afterCreate } = await apiClient.listExecutors();
    const profile = afterCreate
      .find((e) => e.id === executor.id)
      ?.profiles?.find((p) => p.name === "E2E Profile");
    expect(profile).toBeTruthy();
    await runConfig(
      [["delete_executor_profile_kandev", { profile_id: profile!.id }]],
      "Executor profile deleted",
    );
    const { executors: afterDelete } = await apiClient.listExecutors();
    const execAfter = afterDelete.find((e) => e.id === executor.id);
    expect(execAfter?.profiles?.find((p) => p.id === profile!.id)).toBeUndefined();
  });
});
