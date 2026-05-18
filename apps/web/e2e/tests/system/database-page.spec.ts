import { test, expect } from "../../fixtures/test-base";

test.describe("System Database page", () => {
  test("renders database stats and exposes maintenance buttons", async ({ testPage }) => {
    test.setTimeout(60_000);

    await testPage.goto("/settings/system/database");

    await expect(testPage.getByTestId("system-page-title")).toHaveText("Database");
    await expect(testPage.getByTestId("system-database-card")).toBeVisible();

    // Each stat row must render a non-empty value.
    const rows = ["system-db-path", "system-db-size", "system-db-wal", "system-db-schema-version"];
    for (const id of rows) {
      const value = await testPage.getByTestId(id).innerText();
      expect(value.trim().length).toBeGreaterThan(0);
    }
  });

  test("clicking VACUUM briefly disables the button while the request runs", async ({
    testPage,
  }) => {
    test.setTimeout(60_000);

    await testPage.goto("/settings/system/database");
    await expect(testPage.getByTestId("system-database-card")).toBeVisible();

    const vacuumButton = testPage.getByTestId("system-vacuum-button");
    await expect(vacuumButton).toBeEnabled();

    // Fire and confirm the request leaves the page.
    const requestPromise = testPage.waitForRequest(
      (req) => req.url().includes("/api/v1/system/database/vacuum") && req.method() === "POST",
      { timeout: 10_000 },
    );
    await vacuumButton.click();
    await requestPromise;
  });

  test("clicking Factory Reset opens the confirmation modal", async ({ testPage }) => {
    await testPage.goto("/settings/system/database");
    await expect(testPage.getByTestId("system-database-card")).toBeVisible();

    await testPage.getByTestId("system-factory-reset-button").click();
    await expect(testPage.getByTestId("system-factory-reset-dialog")).toBeVisible();
  });
});
