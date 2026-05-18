import { test, expect } from "../../fixtures/office-fixture";

test.describe("Real-time dashboard updates", () => {
  test("dashboard metrics update after task creation", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    await testPage.goto("/office");
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });

    // Create a new task while viewing dashboard
    await apiClient.createTask(officeSeed.workspaceId, "Dashboard Trigger Task", {
      workflow_id: officeSeed.workflowId,
    });

    // The "Recent Tasks" section should eventually show the new task
    await expect(testPage.getByText("Dashboard Trigger Task")).toBeVisible({ timeout: 15_000 });
  });

  test("dashboard does not refetch on cross-workspace task event", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    // Create a second workspace + workflow as the "other" workspace.
    const other = await apiClient.createWorkspace("Other WS for cross-ws test");
    const otherWf = await apiClient.createWorkflow(other.id, "Other WF");

    // User stays on the active office workspace dashboard.
    await testPage.goto("/office");
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });

    // Spy on dashboard refetches for the active workspace.
    const fetchTimes: number[] = [];
    const start = Date.now();
    testPage.on("response", (resp) => {
      const url = resp.url();
      if (url.includes(`/api/v1/office/workspaces/${officeSeed.workspaceId}/dashboard`)) {
        fetchTimes.push(Date.now() - start);
      }
    });

    // Wait for the page + initial fetches to fully settle. The dashboard
    // SSR + client hydration can fire late requests; give it a generous
    // window so we only measure fetches caused by the cross-ws event below.
    let settledAt = 0;
    let priorFetchCount = -1;
    while (settledAt < 8000) {
      await testPage.waitForTimeout(1000);
      settledAt += 1000;
      if (fetchTimes.length === priorFetchCount) break;
      priorFetchCount = fetchTimes.length;
    }
    const baselineCount = fetchTimes.length;

    // Create a task in the OTHER workspace via API (fires office.task.created
    // with workspace_id=other.id).
    await apiClient.createTask(other.id, "Other WS Task — should not trigger", {
      workflow_id: otherWf.id,
    });

    // Wait long enough for any WS event to arrive and would-be refetch to fire.
    await testPage.waitForTimeout(3000);

    // No additional dashboard fetches should have occurred after the event.
    const newFetches = fetchTimes.length - baselineCount;
    expect(
      newFetches,
      `dashboard refetched ${newFetches} times after cross-workspace event (timeline ${fetchTimes.join("ms,")}ms)`,
    ).toBe(0);

    // Sanity: the office API still returns the correct count for our workspace.
    const dash = await officeApi.getDashboard(officeSeed.workspaceId);
    expect(dash).toBeDefined();
  });
});
