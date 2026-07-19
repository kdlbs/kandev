import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("MCP-created task agent profile default", () => {
  test("workspace-default mode routes an omitted-profile subtask to the workspace profile", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const workflow = (await apiClient.listWorkflows(seedData.workspaceId)).workflows.find(
      (candidate) => candidate.id === seedData.workflowId,
    );
    const startStep = seedData.steps.find((step) => step.id === seedData.startStepId);
    expect(workflow?.agent_profile_id).toBeFalsy();
    expect(startStep?.agent_profile_id).toBeFalsy();

    const { agents } = await apiClient.listAgents();
    const workspaceProfile = await apiClient.createAgentProfile(
      agents[0].id,
      "MCP Workspace Default E2E",
      { model: "mock-slow" },
    );
    expect(workspaceProfile.id).not.toBe(seedData.agentProfileId);
    await apiClient.updateWorkspace(seedData.workspaceId, {
      default_agent_profile_id: workspaceProfile.id,
    });

    await testPage.goto("/settings/general/task-actions");
    const workspaceDefault = testPage.getByRole("radio", {
      name: "Workspace default profile",
    });
    await expect(workspaceDefault).not.toBeChecked();
    await workspaceDefault.click();
    await expect(workspaceDefault).toBeChecked();
    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.mcp_task_agent_profile_default)
      .toBe("workspace_default");

    await testPage.reload();
    await expect(workspaceDefault).toBeChecked();

    const subtaskTitle = "Workspace Default MCP Subtask E2E";
    const script = [
      'e2e:thinking("Creating workspace-default subtask...")',
      "e2e:delay(100)",
      `e2e:mcp:kandev:create_task_kandev({"parent_id":"self","title":"${subtaskTitle}","description":"E2E omitted-profile workspace-default subtask"})`,
      "e2e:delay(100)",
      'e2e:message("Done.")',
    ].join("\n");

    const parent = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Workspace Default MCP Parent E2E",
      seedData.agentProfileId,
      {
        description: script,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${parent.id}`);
    await new SessionPage(testPage).waitForLoad();

    const parentSessions = await apiClient.listTaskSessions(parent.id);
    expect(parentSessions.sessions[0]?.agent_profile_id).toBe(seedData.agentProfileId);

    let subtaskId: string | undefined;
    await expect
      .poll(
        async () => {
          const { tasks } = await apiClient.listTasks(seedData.workspaceId);
          subtaskId = tasks.find((task) => task.title === subtaskTitle)?.id;
          return subtaskId;
        },
        { timeout: 60_000, message: "Omitted-profile MCP subtask should be created" },
      )
      .toBeTruthy();

    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(subtaskId!);
          return sessions[0]?.agent_profile_id;
        },
        { timeout: 30_000, message: "MCP subtask should start with the workspace profile" },
      )
      .toBe(workspaceProfile.id);
  });
});
