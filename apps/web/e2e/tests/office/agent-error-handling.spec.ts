import { test, expect } from "../../fixtures/office-fixture";

/**
 * E2E coverage for office-agent-error-handling (the v1 design):
 *   - One terminal failure surfaces in the inbox as agent_run_failed
 *     and on the dashboard agent card subtitle.
 *   - Three consecutive failures (default threshold = 3) auto-pause
 *     the agent and consolidate the inbox into a single
 *     agent_paused_after_failures row.
 *   - Mark fixed on the consolidated row clears the pause and the
 *     inbox empties (no entries left for that agent).
 *   - The sidebar reflects the paused state on the agent row.
 *
 * The test harness exposes `_test/agent-failures` to seed failures
 * deterministically — no real agent is launched.
 */

test.describe("Office agent error handling", () => {
  test("a single terminal failure surfaces in the inbox", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "single-failure", {
      workflow_id: officeSeed.workflowId,
    });
    await apiClient.rawRequest("PATCH", `/api/v1/office/tasks/${task.id}`, {
      assignee_agent_profile_id: officeSeed.agentId,
    });

    await apiClient.seedAgentFailure({
      taskId: task.id,
      agentProfileId: officeSeed.agentId,
      errorMessage: "model not supported",
    });

    await testPage.goto(`/office/inbox`);

    // Per-task entry shows up below threshold.
    await expect(testPage.getByTestId("inbox-item-agent_run_failed").first()).toBeVisible({
      timeout: 10_000,
    });
    // Mark fixed button is present.
    await expect(testPage.getByTestId("inbox-mark-fixed-agent_run_failed").first()).toBeVisible();
  });

  test("3 consecutive failures auto-pause and consolidate the inbox", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    // Three different tasks, three failures across them. Threshold
    // defaults to 3 → the third failure pauses the agent.
    for (let i = 0; i < 3; i++) {
      const task = await apiClient.createTask(officeSeed.workspaceId, `multi-failure-${i}`, {
        workflow_id: officeSeed.workflowId,
      });
      await apiClient.rawRequest("PATCH", `/api/v1/office/tasks/${task.id}`, {
        assignee_agent_profile_id: officeSeed.agentId,
      });
      await apiClient.seedAgentFailure({
        taskId: task.id,
        agentProfileId: officeSeed.agentId,
        errorMessage: `boom-${i}`,
      });
    }

    await testPage.goto(`/office/inbox`);

    // The consolidated paused-agent entry appears.
    await expect(
      testPage.getByTestId("inbox-item-agent_paused_after_failures").first(),
    ).toBeVisible({ timeout: 10_000 });
    // The per-task entries are hidden once the agent is paused
    // (consolidation rule).
    await expect(testPage.getByTestId("inbox-item-agent_run_failed")).toHaveCount(0);
    // Sidebar reflects the paused state on the agent row.
    await expect(testPage.getByTestId("sidebar-agent-paused-badge").first()).toBeVisible();
  });

  test("mark fixed on the paused-agent entry clears the inbox", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    // Pre-seed the same way as the previous test: three failures
    // across three different tasks → agent auto-paused.
    for (let i = 0; i < 3; i++) {
      const task = await apiClient.createTask(officeSeed.workspaceId, `mark-fixed-${i}`, {
        workflow_id: officeSeed.workflowId,
      });
      await apiClient.rawRequest("PATCH", `/api/v1/office/tasks/${task.id}`, {
        assignee_agent_profile_id: officeSeed.agentId,
      });
      await apiClient.seedAgentFailure({
        taskId: task.id,
        agentProfileId: officeSeed.agentId,
        errorMessage: "stuck",
      });
    }

    await testPage.goto(`/office/inbox`);

    const pausedRow = testPage.getByTestId("inbox-item-agent_paused_after_failures").first();
    await expect(pausedRow).toBeVisible({ timeout: 10_000 });

    // Click Mark fixed.
    await testPage.getByTestId("inbox-mark-fixed-agent_paused_after_failures").first().click();

    // After dismissal + WS refetch, the paused entry is gone.
    await expect(testPage.getByTestId("inbox-item-agent_paused_after_failures")).toHaveCount(0, {
      timeout: 10_000,
    });
    // Sidebar paused badge clears too.
    await expect(testPage.getByTestId("sidebar-agent-paused-badge")).toHaveCount(0);
  });
});
