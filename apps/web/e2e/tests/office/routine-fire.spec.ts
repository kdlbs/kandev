import { test, expect } from "../../fixtures/office-fixture";

/**
 * Routine fire → routine_dispatch run → UI deeplink chain.
 *
 * Office routines schedule recurring work via cron triggers; each
 * fire enqueues a `routine_dispatch` run whose payload carries the
 * routine_id. The runs UI surfaces the Linked column with a
 * deeplink back to `/office/routines/<id>`. This spec exercises
 * the contract end-to-end:
 *
 *   1. Create a routine.
 *   2. Seed a `routine_dispatch` run referencing it via the test
 *      harness (the dispatcher path is too heavyweight for CI).
 *   3. Verify the runs UI renders the routine deeplink and that
 *      clicking it lands on /office/routines/<id>.
 */
test.describe("Routine fire UI chain", () => {
  test("routine_dispatch run links back to the routine", async ({
    apiClient,
    officeApi,
    officeSeed,
    testPage,
  }) => {
    test.setTimeout(60_000);

    const routine = (await officeApi.createRoutine(officeSeed.workspaceId, {
      name: "E2E Routine Fire",
    })) as { id: string };
    expect(routine.id).toBeTruthy();

    const run = await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      reason: "routine_dispatch",
      status: "finished",
      routineId: routine.id,
    });

    await testPage.goto(`/office/agents/${officeSeed.agentId}/runs`);
    const row = testPage.getByTestId(`agent-run-row-${run.run_id}`);
    await expect(row).toBeVisible({ timeout: 10_000 });

    const deeplink = row.locator(`a[href="/office/routines/${routine.id}"]`);
    await expect(deeplink).toBeVisible({ timeout: 5_000 });

    await deeplink.click();
    await testPage.waitForURL(`**/office/routines/${routine.id}`);
    await expect(testPage.getByText(/E2E Routine Fire/).first()).toBeVisible({ timeout: 10_000 });
  });
});
