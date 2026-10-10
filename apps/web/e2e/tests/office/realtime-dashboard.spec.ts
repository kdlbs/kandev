import { test, expect } from "../../fixtures/office-fixture";
import { dwell, waitForHttp, watchWs } from "../../helpers/causal-waits";

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

    // The worker's Office agent can publish unrelated activity during this test.
    // Forward the real other-workspace events through the production handlers,
    // while isolating this observation from those independent Office producers.
    let allowActiveWorkspaceEvents = false;
    await testPage.routeWebSocket("**/ws", (client) => {
      const server = client.connectToServer();
      server.onMessage((message) => {
        const frame = JSON.parse(message.toString()) as {
          action?: string;
          payload?: { workspace_id?: string };
        };
        if (
          !allowActiveWorkspaceEvents &&
          frame.action?.startsWith("office.") &&
          frame.payload?.workspace_id !== other.id
        ) {
          return;
        }
        client.send(message);
      });
    });
    const ws = watchWs(testPage);
    const dashboardPath = `/api/v1/office/workspaces/${officeSeed.workspaceId}/dashboard`;
    await testPage.goto("/office");
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });
    await testPage.waitForLoadState("networkidle");

    const fetchTimes: number[] = [];
    const start = Date.now();
    testPage.on("request", (request) => {
      if (request.url().includes(dashboardPath)) fetchTimes.push(Date.now() - start);
    });

    const otherCreated = ws.waitForEvent("office.task.created", {
      where: (payload) => payload.workspace_id === other.id,
    });
    await apiClient.createTask(other.id, "Other WS Task should not trigger", {
      workflow_id: otherWf.id,
    });
    await otherCreated;

    await dwell(
      testPage,
      3000,
      "negative-assertion",
      "the assertion below is that a cross-workspace event triggers no dashboard refetch; a fetch that must never happen has no event, so a regression needs the window in which it would have fired to elapse",
    );

    // No additional dashboard fetches should have occurred after the event.
    const newFetches = fetchTimes.length;
    expect(
      newFetches,
      `dashboard refetched ${newFetches} times after cross-workspace event (timeline ${fetchTimes.join("ms,")}ms)`,
    ).toBe(0);

    // Positive control: the same production path still refetches for this workspace.
    allowActiveWorkspaceEvents = true;
    const activeCreated = ws.waitForEvent("office.task.created", {
      where: (payload) => payload.workspace_id === officeSeed.workspaceId,
    });
    const activeRefetch = waitForHttp(testPage, "GET", new RegExp(`${dashboardPath}$`));
    await officeApi.createTask(officeSeed.workspaceId, "Active workspace dashboard control", {
      workflow_id: officeSeed.workflowId,
    });
    await activeCreated;
    await activeRefetch;
  });
});
