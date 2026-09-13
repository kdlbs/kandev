import { expect, test } from "../../fixtures/test-base";

test("mobile Stats exposes a touch-sized retry and recovers a failed section", async ({
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
        headers: { "Retry-After": "5" },
        contentType: "application/json",
        json: { error: "statistics are busy", error_code: "analytics_busy" },
      });
      return;
    }
    await route.continue();
  });

  await testPage.goto(`/stats?workspaceId=${seedData.workspaceId}`);
  const status = testPage.getByRole("status").filter({ hasText: "Statistics" }).first();
  await expect(status).toBeVisible({ timeout: 10_000 });
  const retry = status.getByRole("button", { name: "Retry", exact: true });
  const retryBox = await retry.boundingBox();
  if (!retryBox) throw new Error("mobile Stats retry control is not visible");
  expect(retryBox.height).toBeGreaterThanOrEqual(44);

  await retry.tap();
  await expect.poll(() => dailyRequests).toBeGreaterThanOrEqual(2);
  await expect(status).toHaveCount(0, { timeout: 10_000 });
  await expect(testPage.getByRole("button", { name: "Copy Stats", exact: true })).toBeEnabled({
    timeout: 10_000,
  });
  expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
    await testPage.evaluate(() => document.documentElement.clientWidth),
  );
});
