import { test, expect } from "../../fixtures/test-base";

test.describe("Mobile System Database page", () => {
  test("renders database stats and maintenance controls", async ({ testPage }) => {
    await testPage.goto("/settings/system/database");

    await expect(testPage.getByTestId("system-page-title")).toHaveText("Database");
    await expect(testPage.getByTestId("system-database-card")).toBeVisible();

    // The default E2E backend uses SQLite, so SQLite-only maintenance buttons render.
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
});
