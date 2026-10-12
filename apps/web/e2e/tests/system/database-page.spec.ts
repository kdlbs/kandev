import { test, expect } from "../../fixtures/test-base";
import {
  DATABASE_SETTINGS_ROUTE,
  expectDatabaseControls,
  routeDatabaseStatsSequence,
} from "../../helpers/database-stats";

test.describe("System Database page", () => {
  test("renders database stats and exposes maintenance buttons", async ({ testPage }) => {
    test.setTimeout(60_000);

    await testPage.goto("/settings/system/data-storage?tab=database");

    await expect(testPage.getByTestId("system-page-title")).toHaveText("Data & Logs");
    await expect(testPage.getByTestId("system-database-card")).toBeVisible();

    // Each stat row must render a non-empty value.
    const rows = [
      "system-db-driver",
      "system-db-path",
      "system-db-size",
      "system-db-wal",
      "system-db-schema-version",
    ];
    for (const id of rows) {
      const value = await testPage.getByTestId(id).innerText();
      expect(value.trim().length).toBeGreaterThan(0);
    }
  });

  test("clicking VACUUM briefly disables the button while the request runs", async ({
    testPage,
  }) => {
    test.setTimeout(60_000);

    await testPage.goto("/settings/system/data-storage?tab=database");
    await expect(testPage.getByTestId("system-database-card")).toBeVisible();

    const vacuumButton = testPage.getByTestId("system-vacuum-button");
    await expect(vacuumButton).toBeEnabled();
    await expect(vacuumButton).toHaveAttribute("data-state", "idle");

    // Fire and confirm the request leaves the page.
    const requestPromise = testPage.waitForRequest(
      (req) => req.url().includes("/api/v1/system/database/vacuum") && req.method() === "POST",
      { timeout: 10_000 },
    );
    await vacuumButton.click();
    await requestPromise;

    // After completion the button switches to the success state (green check
    // + "Done"). useActionFeedback holds the pending state for at least
    // 350ms, so the success transition is observable.
    await expect(vacuumButton).toHaveAttribute("data-state", "success", { timeout: 5_000 });
    // ...and reverts back to idle after the auto-reset window (~1.8s).
    await expect(vacuumButton).toHaveAttribute("data-state", "idle", { timeout: 5_000 });
  });

  test("WAL row exposes an info tooltip trigger", async ({ testPage }) => {
    await testPage.goto("/settings/system/data-storage?tab=database");
    await expect(testPage.getByTestId("system-database-card")).toBeVisible();
    await expect(testPage.getByTestId("system-db-wal-info")).toBeVisible();
  });

  test("clicking Factory Reset opens the confirmation modal", async ({ testPage }) => {
    await testPage.goto("/settings/system/data-storage?tab=database");
    await expect(testPage.getByTestId("system-database-card")).toBeVisible();

    await testPage.getByTestId("system-factory-reset-button").click();
    await expect(testPage.getByTestId("system-factory-reset-dialog")).toBeVisible();
  });

  // @covers AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001.3 AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001.5 AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-002.3
  test("keeps metadata through reloads and recovers a stale logical snapshot", async ({
    testPage,
  }) => {
    const scenario = await routeDatabaseStatsSequence(testPage);
    try {
      await testPage.goto(DATABASE_SETTINGS_ROUTE);
      await expect(testPage.getByTestId("system-db-logical-stats-status")).toContainText(
        "Measuring logical totals",
      );
      await expectDatabaseControls(testPage);

      scenario.showRefreshing();
      await testPage.reload();
      await expect(testPage.getByTestId("system-db-logical-stats-status")).toContainText(
        "Updating",
      );
      await expectDatabaseControls(testPage);

      scenario.showStale();
      await testPage.reload();
      await expect(testPage.getByTestId("system-db-logical-stats-status")).toContainText(
        "These values may be stale",
      );
      await expect(testPage.getByTestId("system-db-metadata-stale")).toBeVisible();
      await expectDatabaseControls(testPage);

      scenario.recover();
      const retryRequest = testPage.waitForRequest(
        (request) =>
          request.url().includes("/api/v1/system/database/refresh") && request.method() === "POST",
      );
      await testPage.getByTestId("system-db-logical-stats-retry").click();
      await retryRequest;
      await expect(testPage.getByTestId("system-db-logical-stats-status")).toContainText(
        "Logical totals measured at",
      );
      await expectDatabaseControls(testPage);
    } finally {
      await scenario.remove();
    }
  });
});
