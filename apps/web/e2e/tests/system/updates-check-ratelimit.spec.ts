import { test, expect } from "../../fixtures/test-base";

test.describe("System Updates — rate-limited check", () => {
  test("clicking Check now while rate-limited surfaces a 'Try again in <N>s' error", async ({
    testPage,
  }) => {
    test.setTimeout(30_000);

    // Provide a base payload for the initial fetch so the page renders cleanly.
    await testPage.route("**/api/v1/system/updates", (route) => {
      void route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          current: "v1.0.0",
          latest: "v1.0.0",
          latest_url: "",
          latest_checked_at: new Date().toISOString(),
          update_available: false,
        }),
      });
    });

    // The /updates/check endpoint returns a rate-limited 429.
    await testPage.route("**/api/v1/system/updates/check", (route) => {
      void route.fulfill({
        status: 429,
        contentType: "application/json",
        body: JSON.stringify({
          error: "Already checked recently. Retry after 25 seconds.",
          retry_after_seconds: 25,
        }),
      });
    });

    await testPage.goto("/settings/system/updates");
    await expect(testPage.getByTestId("system-updates-card")).toBeVisible();

    await testPage.getByTestId("system-updates-check").click();

    const errorMsg = testPage.getByTestId("system-updates-error");
    await expect(errorMsg).toBeVisible({ timeout: 10_000 });
    await expect(errorMsg).toContainText(/try again in 25s/i);
  });
});
