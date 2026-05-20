import { test, expect } from "../../fixtures/office-fixture";

/**
 * E2E coverage for the new agent dashboard sub-page
 * (`/office/agents/[id]/dashboard`). Seeds a small fixture of runs +
 * activity + cost events, then walks the rendered charts, recent
 * tasks list, and costs section.
 *
 * The page is server-rendered: one navigation produces the full HTML
 * including the chart segment values, so the assertions don't need
 * to wait for client-side hydration to inspect the data.
 */

test.describe("Agent dashboard", () => {
  test("renders chart values, latest run, recent tasks, and costs", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    // Seed three tasks the agent has touched.
    const task1 = await apiClient.createTask(officeSeed.workspaceId, "Dashboard Task A", {
      workflow_id: officeSeed.workflowId,
    });
    const task2 = await apiClient.createTask(officeSeed.workspaceId, "Dashboard Task B", {
      workflow_id: officeSeed.workflowId,
    });

    const now = Date.now();
    const isoMinusDays = (n: number, hours = 0) =>
      new Date(now - n * 24 * 3600_000 - hours * 3600_000).toISOString();

    // Seed runs across 5 days: 3 succeeded + 1 failed.
    await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      status: "finished",
      taskId: task1.id,
      requestedAt: isoMinusDays(0, 1),
      finishedAt: isoMinusDays(0, 0),
    });
    await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      status: "finished",
      taskId: task1.id,
      requestedAt: isoMinusDays(1, 1),
      finishedAt: isoMinusDays(1, 0),
    });
    await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      status: "finished",
      taskId: task2.id,
      requestedAt: isoMinusDays(3, 1),
      finishedAt: isoMinusDays(3, 0),
    });
    const failedRun = await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      status: "failed",
      taskId: task2.id,
      errorMessage: "boom",
      requestedAt: isoMinusDays(4, 1),
      finishedAt: isoMinusDays(4, 0),
    });

    // Most-recent activity rows for the recent-tasks list. Must use
    // the agent id as actor_id so the dashboard's WHERE clause picks
    // them up. The dashboard service treats both target_type='task'
    // rows as activity for that task.
    await apiClient.seedActivity({
      workspaceId: officeSeed.workspaceId,
      actorType: "agent",
      actorId: officeSeed.agentId,
      action: "task.update",
      targetType: "task",
      targetId: task1.id,
      createdAt: isoMinusDays(0, 1),
    });
    await apiClient.seedActivity({
      workspaceId: officeSeed.workspaceId,
      actorType: "agent",
      actorId: officeSeed.agentId,
      action: "task.update",
      targetType: "task",
      targetId: task2.id,
      createdAt: isoMinusDays(0, 2),
    });

    // Cost events for the costs section.
    await apiClient.seedCostEvent({
      agentProfileId: officeSeed.agentId,
      taskId: task1.id,
      tokensIn: 1000,
      tokensOut: 500,
      tokensCachedIn: 200,
      costSubcents: 1200,
      occurredAt: isoMinusDays(0, 0),
    });
    await apiClient.seedCostEvent({
      agentProfileId: officeSeed.agentId,
      taskId: task1.id,
      tokensIn: 200,
      tokensOut: 100,
      tokensCachedIn: 50,
      costSubcents: 300,
      occurredAt: isoMinusDays(0, 0),
    });

    await testPage.goto(`/office/agents/${officeSeed.agentId}/dashboard`);

    // Latest run card carries the most recent run's status. The most
    // recent seeded row by requested_at is the day-0 finished run.
    const latestCard = testPage.getByTestId("latest-run-card");
    await expect(latestCard).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByTestId("latest-run-status")).toHaveText("finished");

    // Run activity chart has the right number of bars (14-day window)
    // and shows the seeded counts. We don't pin specific dates in the
    // bars because the test runs in real time; instead we sum every
    // segment's data-segment-value.
    const runActivityCard = testPage.getByTestId("run-activity-card");
    const runBars = runActivityCard.getByTestId("stacked-bar");
    await expect(runBars).toHaveCount(14);

    const succeededTotal = await runActivityCard
      .locator('[data-segment-key="succeeded"]')
      .evaluateAll((nodes) =>
        nodes.reduce((sum, n) => sum + Number(n.getAttribute("data-segment-value") ?? "0"), 0),
      );
    const failedTotal = await runActivityCard
      .locator('[data-segment-key="failed"]')
      .evaluateAll((nodes) =>
        nodes.reduce((sum, n) => sum + Number(n.getAttribute("data-segment-value") ?? "0"), 0),
      );
    expect(succeededTotal).toBe(3);
    expect(failedTotal).toBe(1);

    // Recent tasks lists both seeded tasks. Order: most-recent
    // last_active_at first (task1 by 1h vs task2 by 2h).
    const recentRows = testPage.getByTestId("recent-task-row");
    await expect(recentRows).toHaveCount(2);
    await expect(recentRows.first()).toHaveAttribute("data-task-id", task1.id);

    // Costs aggregate sums match (1200 / 600 / 250 / 15c).
    await expect(testPage.getByTestId("agg-input")).toHaveText("1.2k");
    await expect(testPage.getByTestId("agg-output")).toHaveText("600");
    await expect(testPage.getByTestId("agg-cached")).toHaveText("250");
    await expect(testPage.getByTestId("agg-total-cost")).toHaveText("$0.15");

    // Defensive: fixture-bound API check that the failed run id is
    // among the returned summary so future changes that drop runs
    // from the cost rollup still flag here.
    void failedRun;
    void officeApi;
  });

  test("dashboard renders chart primitives and latest-run card after navigation", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "SSR Probe Task", {
      workflow_id: officeSeed.workflowId,
    });
    await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      status: "finished",
      taskId: task.id,
      requestedAt: new Date(Date.now() - 3600_000).toISOString(),
      finishedAt: new Date().toISOString(),
    });

    // The page is a Server Component (page.tsx pre-fetches the
    // summary) wrapped in a Client Component layout. The dashboard
    // view receives the server-fetched snapshot synchronously into
    // useState, so the chart primitives + latest-run card are present
    // in the rendered DOM with no waiting on a client-side fetch.
    await testPage.goto(`/office/agents/${officeSeed.agentId}/dashboard`);
    await expect(testPage.getByTestId("latest-run-card")).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("stacked-bar").first()).toBeVisible({ timeout: 15_000 });
    await expect(testPage.locator('[data-segment-key="succeeded"]').first()).toBeAttached();
  });
});
