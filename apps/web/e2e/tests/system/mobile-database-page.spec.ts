import { test, expect } from "../../fixtures/test-base";
import {
  DATABASE_SETTINGS_ROUTE,
  expectDatabaseControls,
  routeDatabaseStatsSequence,
} from "../../helpers/database-stats";

test.describe("Mobile System Database page", () => {
  test("renders database stats and maintenance controls", async ({ testPage }) => {
    await testPage.goto("/settings/system/data-storage?tab=database");

    await expect(testPage.getByTestId("system-page-title")).toHaveText("Data & Logs");
    await expect(testPage.getByTestId("system-database-card")).toBeVisible();

    // Maintenance buttons are only rendered for SQLite; this spec assumes a SQLite backend.
    for (const id of [
      "system-db-driver",
      "system-db-size",
      "system-db-schema-version",
      "system-vacuum-button",
      "system-optimize-button",
      "system-factory-reset-button",
    ]) {
      await expect(testPage.getByTestId(id)).toBeVisible();
    }
  });

  test("shows stale measurements, keeps maintenance reachable, and retries after reload on a phone", async ({
    testPage: page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    const scenario = await routeDatabaseStatsSequence(page);
    try {
      await page.goto(DATABASE_SETTINGS_ROUTE);
      await expect(page.getByTestId("system-db-logical-stats-status")).toContainText(
        "Measuring logical totals",
      );
      await expectDatabaseControls(page);

      scenario.showRefreshing();
      await page.reload();
      await expect(page.getByTestId("system-db-logical-stats-status")).toContainText("Updating");
      await expectDatabaseControls(page);

      scenario.showStale();
      await page.reload();
      await expect(page.getByTestId("system-db-logical-stats-status")).toContainText(
        "These values may be stale",
      );
      await expect(page.getByTestId("system-db-metadata-stale")).toBeVisible();
      await expectDatabaseControls(page);

      scenario.recover();
      const retry = page.getByTestId("system-db-logical-stats-retry");
      const box = await retry.boundingBox();
      expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
      const retryRequest = page.waitForRequest(
        (request) =>
          request.url().includes("/api/v1/system/database/refresh") && request.method() === "POST",
      );
      await retry.tap();
      await retryRequest;
      await expect(page.getByTestId("system-db-logical-stats-status")).toContainText(
        "Logical totals measured at",
      );
      await expectDatabaseControls(page);
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      ).toBe(true);
    } finally {
      await scenario.remove();
    }
  });
});
