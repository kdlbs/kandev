import { test, expect } from "../../fixtures/office-fixture";

/**
 * E2E coverage for Wave 2 Agent E — live WebSocket streaming on the
 * run detail page. The run detail snapshot is delivered SSR; while
 * the run is `claimed`, the page subscribes to `run.subscribe` and
 * appends new events via `run.event.appended` without a page reload.
 *
 * Strategy:
 *   - Seed a run with status `claimed` so the live-sync hook engages.
 *   - Seed an initial event so the events log isn't empty when the
 *     page renders (lets us scope assertions to "rows that arrive
 *     after navigation").
 *   - Append a new event via `_test/run-events`; the harness
 *     publishes the matching bus notification, which the gateway
 *     fans out as `run.event.appended` to the open page.
 *   - Assert the new event's row materialises in the events log,
 *     keyed on its seq, without `page.reload()`.
 */
test.describe("Office agent run live", () => {
  test("appends run events over WebSocket without reload", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const seeded = await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      reason: "task_assigned",
      status: "claimed",
      claimedAt: new Date().toISOString(),
    });
    const runId = seeded.run_id;

    // Seed one event so the SSR snapshot already has a row — the
    // events log container uses [data-testid="events-log"] vs the
    // empty-state variant, and we want to land on the populated one
    // before driving live updates.
    await apiClient.seedRunEvent({ runId, eventType: "init", level: "info" });

    await testPage.goto(`/office/agents/${officeSeed.agentId}/runs/${runId}`);

    // Initial render — the seeded "init" row (seq 0) is present.
    await expect(testPage.getByTestId("events-log")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByTestId("events-log-row-0")).toBeVisible();

    // Append a NEW event after the page is mounted. The live-sync
    // hook should pick it up via WS and render it as seq 1.
    await apiClient.seedRunEvent({
      runId,
      eventType: "adapter.invoke",
      level: "info",
    });

    // Without a reload, the new row materialises in the events log.
    await expect(testPage.getByTestId("events-log-row-1")).toBeVisible({
      timeout: 5_000,
    });
    await expect(testPage.getByTestId("events-log-row-1")).toContainText("adapter.invoke");
  });

  test("transitions header status badge to finished on terminal event", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const seeded = await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      reason: "task_assigned",
      status: "claimed",
      claimedAt: new Date().toISOString(),
    });
    const runId = seeded.run_id;

    await testPage.goto(`/office/agents/${officeSeed.agentId}/runs/${runId}`);

    // Header initially shows the running ("claimed") badge.
    await expect(testPage.getByTestId("run-status-badge")).toContainText("claimed", {
      timeout: 10_000,
    });

    // Append a terminal "complete" event over the bus. The hook
    // observes the event_type and flips local status to finished
    // without a snapshot refetch.
    //
    // Poll-retry the seed because the WS `run.subscribe` message is
    // fire-and-forget on the client and the broadcaster only fans an
    // event to clients already in the per-run subscriber map at
    // publish time. If the first seed lands before the backend has
    // processed the subscribe, the event is silently dropped for
    // this client. Re-seeding catches the race once the subscription
    // is registered server-side.
    await expect
      .poll(
        async () => {
          await apiClient.seedRunEvent({
            runId,
            eventType: "complete",
            level: "info",
          });
          return await testPage.getByTestId("run-status-badge").textContent();
        },
        { timeout: 15_000, intervals: [500, 1000, 2000] },
      )
      .toContain("finished");
  });
});
