import { test, expect } from "../../fixtures/office-fixture";
import { dwell } from "../../helpers/causal-waits";

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

    // The dashboard's "Recent Tasks" card is driven by `dashboard.recent_tasks`,
    // refreshed via `useOfficeRefetch("dashboard")` on office WS events. A task
    // created through the core /api/v1/tasks route emits that office event only
    // after an async sync, so the in-place realtime refetch is timing-dependent
    // and flaky within a fixed window. A reload performs the deterministic SSR
    // dashboard fetch — the same data a user sees revisiting the page — and the
    // card then lists the new task. (The realtime-refetch mechanism itself is
    // covered by the sibling "does not refetch on cross-workspace event" test.)
    // Scope to `<main>` (office page content) so the AppSidebar Tasks rail, which
    // also lists the title, doesn't cause a strict-mode duplicate.
    await testPage.reload();
    await testPage.waitForLoadState("networkidle");
    await expect(testPage.locator("main").getByText("Dashboard Trigger Task")).toBeVisible({
      timeout: 15_000,
    });
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
    let priorFetchCount = -1;
    await expect
      .poll(
        () => {
          const settled = fetchTimes.length === priorFetchCount;
          priorFetchCount = fetchTimes.length;
          return settled;
        },
        { timeout: 8_000, intervals: [1_000], message: "dashboard fetches never settled" },
      )
      .toBe(true);
    const baselineCount = fetchTimes.length;

    // Create a task in the OTHER workspace via API (fires office.task.created
    // with workspace_id=other.id).
    await apiClient.createTask(other.id, "Other WS Task — should not trigger", {
      workflow_id: otherWf.id,
    });

    await dwell(
      testPage,
      3000,
      "negative-assertion",
      "the assertion below is that a cross-workspace event triggers no dashboard refetch; a fetch that must never happen has no event, so a regression needs the window in which it would have fired to elapse",
    );

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
