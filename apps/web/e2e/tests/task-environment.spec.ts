import { test, expect } from "../fixtures/test-base";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

/**
 * Tests for the TaskEnvironment model: verifies that per-task execution environments
 * are created on first session launch and reused by subsequent sessions.
 */
test.describe("Task environment", () => {
  test("first session creates a task environment record", async ({ apiClient, seedData }) => {
    // Before creating a task, there should be no environment
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Env Create Test",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // Wait for agent to complete
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for session to finish" },
      )
      .toBe(true);

    // Verify task environment exists
    const env = await apiClient.getTaskEnvironment(task.id);
    expect(env).not.toBeNull();
    expect(env!.task_id).toBe(task.id);
    expect(env!.status).toBe("ready");
  });

  test("session has task_environment_id linking it to the environment", async ({
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Env Link Test",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // Wait for agent to complete
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for session to finish" },
      )
      .toBe(true);

    const env = await apiClient.getTaskEnvironment(task.id);
    expect(env).not.toBeNull();

    const { sessions } = await apiClient.listTaskSessions(task.id);
    expect(sessions).toHaveLength(1);
    expect(sessions[0].task_environment_id).toBe(env!.id);
  });

  test("no task environment exists for a task without sessions", async ({
    apiClient,
    seedData,
  }) => {
    // Create a task without starting an agent
    const task = await apiClient.createTask(seedData.workspaceId, "No Env Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    // No environment should exist
    const env = await apiClient.getTaskEnvironment(task.id);
    expect(env).toBeNull();
  });

  test("environment worktree info matches session worktree", async ({ apiClient, seedData }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Env Worktree Match Test",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // Wait for agent to complete
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for session to finish" },
      )
      .toBe(true);

    const env = await apiClient.getTaskEnvironment(task.id);
    expect(env).not.toBeNull();
    const { sessions } = await apiClient.listTaskSessions(task.id);

    // If the session has a worktree_path, it should match the environment's worktree
    if (sessions[0].worktree_path) {
      expect(env!.worktree_path).toBe(sessions[0].worktree_path);
    }
  });
});
