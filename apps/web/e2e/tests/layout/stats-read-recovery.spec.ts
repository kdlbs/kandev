import { expect, test } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";

test.describe("Stats section recovery", () => {
  test("keeps successful sections, retries a failed section, and gates Copy Stats", async ({
    testPage,
    seedData,
  }) => {
    let dailyRequests = 0;
    await testPage.route("**/api/v1/**", async (route) => {
      const url = new URL(route.request().url());
      if (url.pathname !== `/api/v1/workspaces/${seedData.workspaceId}/stats/daily-activity`) {
        await route.continue();
        return;
      }
      dailyRequests += 1;
      if (dailyRequests === 1) {
        await route.fulfill({
          status: 503,
          headers: { "Retry-After": "1" },
          contentType: "application/json",
          json: { error: "statistics are busy", error_code: "analytics_busy" },
        });
        return;
      }
      await route.continue();
    });

    const failedDailyRead = waitForHttp(
      testPage,
      "GET",
      new RegExp(`^/api/v1/workspaces/${seedData.workspaceId}/stats/daily-activity$`),
      { predicate: (response) => response.status() === 503 },
    );
    await testPage.goto(`/stats?workspaceId=${seedData.workspaceId}`);
    await failedDailyRead;
    const activityStatus = testPage.getByRole("status").filter({ hasText: "Statistics" }).first();
    await expect(activityStatus).toBeVisible();
    await expect(testPage.getByRole("main").getByText("Tasks", { exact: true })).toBeVisible();

    const recoveredDailyRead = waitForHttp(
      testPage,
      "GET",
      new RegExp(`^/api/v1/workspaces/${seedData.workspaceId}/stats/daily-activity$`),
      { predicate: (response) => response.ok() },
    );
    await expect(testPage.getByRole("button", { name: "Copy Stats", exact: true })).toBeDisabled();
    await testPage.getByRole("button", { name: "Retry", exact: true }).click();
    await recoveredDailyRead;
    await expect(activityStatus).toHaveCount(0);
    await expect(testPage.getByRole("button", { name: "Copy Stats", exact: true })).toBeEnabled();
  });
});
