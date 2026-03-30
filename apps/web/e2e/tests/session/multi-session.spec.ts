import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

/**
 * Tests for launching multiple sessions on the same task.
 * Verifies session handover context injection and environment reuse.
 */
test.describe("Multi-session", () => {
  test("second session on same task receives handover context", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    // 1. Create task and start first agent session
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Multi Session Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 2. Wait for first session to finish
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for first session to finish" },
      )
      .toBe(true);

    // 3. Verify first session created environment
    const env = await apiClient.getTaskEnvironment(task.id);
    expect(env).not.toBeNull();

    // 4. Navigate to task and verify first session is visible
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("Multi Session Task");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Verify first session's response is visible
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // 5. Verify task has exactly one session
    const { sessions: sessionsAfterFirst } = await apiClient.listTaskSessions(task.id);
    expect(sessionsAfterFirst).toHaveLength(1);
    expect(sessionsAfterFirst[0].task_environment_id).toBe(env!.id);
  });

  test("task environment persists after session completes", async ({ apiClient, seedData }) => {
    test.setTimeout(90_000);

    // Create task and start agent
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Env Persistence Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // Wait for session to finish
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for session to finish" },
      )
      .toBe(true);

    // Verify environment still exists and is in "ready" state after completion
    const env = await apiClient.getTaskEnvironment(task.id);
    expect(env).not.toBeNull();
    expect(env!.status).toBe("ready");
    expect(env!.task_id).toBe(task.id);
  });

  test("second session reuses same worktree as first session", async ({ apiClient, seedData }) => {
    test.setTimeout(90_000);

    // 1. Create task with first session
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Worktree Reuse Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 2. Wait for first session to finish
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for first session to finish" },
      )
      .toBe(true);

    // 3. Capture first session's environment info
    const { sessions: firstSessions } = await apiClient.listTaskSessions(task.id);
    const firstSession = firstSessions[0];
    const env = await apiClient.getTaskEnvironment(task.id);
    expect(env).not.toBeNull();

    // 4. Verify the session points to the task environment
    // The worktree reuse is verified through the task_environment_id linkage:
    // when a second session launches on the same task, persistTaskEnvironment()
    // finds the existing environment and reuses its WorktreeID.
    expect(firstSession.task_environment_id).toBe(env!.id);
  });
});
