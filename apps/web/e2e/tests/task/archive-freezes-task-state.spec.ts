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
// deterministically: archive a task mid-turn, then cancel the turn via the
// same WS action the chat toolbar's Cancel button sends.
test.describe("Archiving a task freezes its runtime state", () => {
  test("agent.cancel after archive does not resurrect task state to REVIEW", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    const taskTitle = "Archive Freezes State";
    const created = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      taskTitle,
      seedData.agentProfileId,
      {
        description: 'e2e:delay(15000)\ne2e:message("still going")',
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    const taskId = created.id;
    const sessionId = created.session_id;
    expect(sessionId, "createTaskWithAgent should return a session_id").toBeTruthy();

    // Wait for the agent to actually be mid-turn (task promoted to
    // IN_PROGRESS) before archiving — this is the state we assert stays
    // frozen for the rest of the test.
    await expect
      .poll(async () => (await apiClient.getTask(taskId)).state, {
        timeout: 20_000,
        message: "waiting for task to reach IN_PROGRESS before archiving",
      })
      .toBe("IN_PROGRESS");

    // Archive while the agent turn is still in flight (15s delay keeps the
    // mock agent from finishing on its own during this test).
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
