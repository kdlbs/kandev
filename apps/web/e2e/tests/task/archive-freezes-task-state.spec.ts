import { test, expect } from "../../fixtures/test-base";

// Regression: archiving a task must freeze its runtime `state`. Several
// backend reconcile paths (turn completion, turn cancel, failed-start
// fallback, and startup/crash reconciliation) used to write tasks.state to
// REVIEW whenever the *session* looked idle/failed, without checking
// whether the *task* had since been archived. Once a task was archived
// while its session was still active, any of those paths — including a
// plain agent.cancel — silently resurrected the archived task's state,
// which is the "archived task comes back with the wrong state after a
// crash/restart" bug. This test reproduces the live (non-restart) trigger
// deterministically: the E2E harness seeds the persisted RUNNING session
// state, then the test archives the task and cancels the turn via the same WS
// action the chat toolbar's Cancel button sends. Seeding the state avoids
// coupling this regression to the mock executor's startup timing.
test.describe("Archiving a task freezes its runtime state", () => {
  test("agent.cancel after archive does not resurrect task state to REVIEW", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const taskTitle = "Archive Freezes State";
    const { task_id: taskId } = await apiClient.seedTask(seedData.workspaceId, taskTitle, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      state: "IN_PROGRESS",
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(taskId, {
      state: "RUNNING",
      agentProfileId: seedData.agentProfileId,
    });

    // Confirm the seeded task is in the exact state that an in-flight agent
    // turn would have reached before archiving. The test harness seeds this
    // state directly so entering the workflow step cannot launch an agent.
    await expect
      .poll(async () => (await apiClient.getTask(taskId)).state, {
        timeout: 30_000,
        message: "waiting for task to reach IN_PROGRESS before archiving",
      })
      .toBe("IN_PROGRESS");

    // Archive while the seeded session reports an in-flight agent turn.
    await apiClient.archiveTask(taskId);

    // Cancel the now-archived task's turn — the same `agent.cancel` WS
    // action the chat toolbar's Cancel button sends. Service.CancelAgent
    // tolerates a missing/already-stopped execution and still reconciles
    // DB state, which is exactly the path that used to write REVIEW onto
    // an archived task unconditionally.
    await apiClient.wsRequest("agent.cancel", { session_id: sessionId });

    const taskAfterCancel = await apiClient.getTask(taskId);
    expect(taskAfterCancel.state).toBe("IN_PROGRESS");

    // User-facing check: the Tasks list (with archived tasks shown) must
    // reflect the same frozen state — the card must not have been
    // silently moved into a "needs review" bucket.
    // The task intentionally has no repository, so clear any persisted
    // repository filter before opening the list. The shared E2E user can have
    // inherited settings from a preceding test in the same worker.
    await apiClient.saveUserSettings({ repository_ids: [] });
    await testPage.goto("/tasks");
    await testPage.waitForLoadState("networkidle");
    await testPage.getByRole("checkbox", { name: "Show archived" }).click();

    const row = testPage
      .getByTestId("tasks-list-row")
      .filter({ has: testPage.getByTestId("tasks-list-row-title").getByText(taskTitle) });
    await expect(row).toBeVisible({ timeout: 15_000 });
    await expect(row.getByText("Archived")).toBeVisible();

    // Re-check via API right after the UI observed the row, closing out
    // on the precise contract: still IN_PROGRESS, never REVIEW.
    const finalTask = await apiClient.getTask(taskId);
    expect(finalTask.state).toBe("IN_PROGRESS");
  });
});
