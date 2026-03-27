import { test, expect } from "../fixtures/test-base";
import { SessionPage } from "../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

type SessionInfo = {
  id: string;
  agent_profile_id: string;
  executor_profile_id: string;
  state: string;
};

/**
 * Verifies that when an agent creates a subtask via the MCP create_task tool
 * with parent_id="self", the subtask session inherits BOTH the parent session's
 * agent_profile_id AND executor_profile_id.
 *
 * Key invariants tested:
 *   - agent_profile_id: copied from parent.AgentProfileID
 *   - executor_profile_id: copied from parent.ExecutorProfileID
 *
 * Before fix: autoStartTask had no agent profile (workspace has no default),
 * so it skipped launching → subtask had zero sessions.
 * After fix: autoStartTask inherits both IDs via inheritFromParentSession.
 *
 * To make executor_profile_id non-vacuous we set it explicitly on the parent
 * session via createTaskWithAgent + executor_profile_id option.
 */
test.describe("MCP create_task subtask executor inheritance", () => {
  test("MCP-created subtask inherits agent profile and executor profile", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const subtaskTitle = "Inherited Profile Subtask E2E";

    const script = [
      'e2e:thinking("Creating subtask...")',
      "e2e:delay(100)",
      `e2e:mcp:kandev:create_task({"parent_id":"self","title":"${subtaskTitle}","description":"E2E subtask: verify executor and agent profile inheritance"})`,
      "e2e:delay(100)",
      'e2e:message("Done.")',
    ].join("\n");

    // 1. Create an executor + profile so executor_profile_id is non-empty on
    //    the parent session — this makes the inheritance assertion meaningful.
    const executor = await apiClient.createExecutor("E2E Inherit Executor", "local_pc");
    const executorProfile = await apiClient.createExecutorProfile(
      executor.id,
      "E2E Inherit Profile",
    );

    // 2. Create parent task via API with explicit executor_profile_id so the
    //    parent session records it. The mock agent runs the MCP script which
    //    creates the subtask with parent_id="self".
    const parentTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Executor Inherit Parent Task",
      seedData.agentProfileId,
      {
        description: script,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        executor_profile_id: executorProfile.id,
      },
    );
    expect(parentTask.id).toBeTruthy();

    // 3. Navigate to the parent task page and wait for the agent to complete.
    await testPage.goto(`/t/${parentTask.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // 4. Confirm the parent session has both profiles set as expected.
    const parentRaw = await apiClient.rawRequest("GET", `/api/v1/tasks/${parentTask.id}/sessions`);
    const parentData = (await parentRaw.json()) as { sessions: SessionInfo[] };
    const parentSession = parentData.sessions[0];
    expect(
      parentSession?.agent_profile_id,
      `agent_profile_id: got ${parentSession?.agent_profile_id}, want ${seedData.agentProfileId}`,
    ).toBe(seedData.agentProfileId);
    expect(
      parentSession?.executor_profile_id,
      `executor_profile_id: got ${parentSession?.executor_profile_id}, want ${executorProfile.id}; full session: ${JSON.stringify(parentSession)}`,
    ).toBe(executorProfile.id);

    // 5. Locate the subtask created by the MCP call.
    const allTasks = await apiClient.listTasks(seedData.workspaceId);
    const subtask = allTasks.tasks.find((t) => t.title === subtaskTitle);
    expect(subtask).toBeDefined();

    // 6. Verify subtask was auto-started: must have at least one session.
    //    Without the fix, autoStartTask finds no agent profile and skips launch →
    //    the subtask remains sessionless.
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(subtask!.id);
          return sessions.length;
        },
        { timeout: 30_000, message: "Subtask should be auto-started with an inherited session" },
      )
      .toBeGreaterThanOrEqual(1);

    // 7. Wait for the subtask session to reach a terminal state.
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(subtask!.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for subtask session to complete" },
      )
      .toBe(true);

    // 8. Verify the subtask session inherited BOTH profiles from the parent —
    //    proving executor/agent profile inheritance worked correctly.
    const subtaskRaw = await apiClient.rawRequest("GET", `/api/v1/tasks/${subtask!.id}/sessions`);
    const subtaskData = (await subtaskRaw.json()) as { sessions: SessionInfo[] };
    const subtaskSession = subtaskData.sessions[0];

    expect(subtaskSession?.agent_profile_id).toBe(parentSession?.agent_profile_id);
    expect(subtaskSession?.executor_profile_id).toBe(parentSession?.executor_profile_id);
    // Sanity: both are non-empty (so the assertions above are not vacuously true)
    expect(subtaskSession?.agent_profile_id).toBeTruthy();
    expect(subtaskSession?.executor_profile_id).toBeTruthy();
  });
});
