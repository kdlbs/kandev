import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

/**
 * Tests for executor environment reuse across sessions.
 * Verifies that task environments persist across sessions and that
 * the API response includes container_id/sandbox_id fields.
 *
 * Note: E2E tests run with KANDEV_DOCKER_ENABLED=false and KANDEV_MOCK_AGENT=only,
 * so Docker container reuse cannot be tested end-to-end here.
 * Docker reconnect logic is covered by Go unit tests.
 */
test.describe("Executor reuse", () => {
  test("task environment API response includes executor_type field", async ({
    apiClient,
    seedData,
  }) => {
    // Create task with session, wait for completion
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Env Fields Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for session to finish" },
      )
      .toBe(true);

    // Get environment and verify response shape
    const env = await apiClient.getTaskEnvironment(task.id);
    expect(env).not.toBeNull();
    expect(env!.task_id).toBe(task.id);
    expect(env!.status).toBe("ready");
    // executor_type should be present (standalone for mock agent)
    expect(env!.executor_type).toBeDefined();
  });

  test("second session reuses same task environment by default", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    // 1. Create task with first session
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Reuse Env Task",
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
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for first session to finish" },
      )
      .toBe(true);

    // 3. Capture environment
    const envBefore = await apiClient.getTaskEnvironment(task.id);
    expect(envBefore).not.toBeNull();

    // 4. Navigate to task and create second session via dialog
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("Reuse Env Task");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // 5. Open new session dialog and submit
    await session.openNewSessionDialog();
    await expect(session.newSessionDialog()).toBeVisible({ timeout: 5_000 });
    await session.newSessionPromptInput().fill("/e2e:simple-message");
    await session.newSessionStartButton().click();
    await expect(session.newSessionDialog()).not.toBeVisible({ timeout: 10_000 });

    // 6. Wait for second session
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.length;
        },
        { timeout: 30_000, message: "Waiting for second session to appear" },
      )
      .toBe(2);

    // 7. Verify same environment ID (default reuse behavior)
    const envAfter = await apiClient.getTaskEnvironment(task.id);
    expect(envAfter).not.toBeNull();
    expect(envAfter!.id).toBe(envBefore!.id);

    // 8. Both sessions reference the same environment
    const { sessions: allSessions } = await apiClient.listTaskSessions(task.id);
    for (const s of allSessions) {
      expect(s.task_environment_id).toBe(envBefore!.id);
    }
  });

  test("worktree executor sessions do not show the managed environment popover", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Reset Env Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );

    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for session to finish" },
      )
      .toBe(true);

    const envBefore = await apiClient.getTaskEnvironment(task.id);
    expect(envBefore).not.toBeNull();
    expect(envBefore!.executor_type).toBe("worktree");

    // Navigate to task
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("Reset Env Task");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(testPage.getByTestId("executor-settings-button")).toHaveCount(0);
  });
});
