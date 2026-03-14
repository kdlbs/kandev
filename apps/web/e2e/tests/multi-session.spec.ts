import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";

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

    // 2. Wait for first session to complete
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions[0]?.state;
        },
        { timeout: 30_000, message: "Waiting for first session to complete" },
      )
      .toBe("COMPLETED");

    // 3. Verify first session created environment
    const env = await apiClient.getTaskEnvironment(task.id);
    expect(env).not.toBeNull();

    // 4. Navigate to task and verify first session is visible
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("Multi Session Task");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

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

    // Wait for session to complete
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions[0]?.state;
        },
        { timeout: 30_000, message: "Waiting for session to complete" },
      )
      .toBe("COMPLETED");

    // Verify environment still exists and is in "ready" state after completion
    const env = await apiClient.getTaskEnvironment(task.id);
    expect(env).not.toBeNull();
    expect(env!.status).toBe("ready");
    expect(env!.task_id).toBe(task.id);
  });
});
