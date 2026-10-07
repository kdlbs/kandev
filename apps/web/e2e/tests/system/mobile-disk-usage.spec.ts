import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { waitForHttp } from "../../helpers/causal-waits";

test.describe("System Status disk usage on mobile", () => {
  test("shows the breakdown and keeps refresh POST then GET working on a phone", async ({
    testPage,
  }) => {
    test.setTimeout(60_000);
    await testPage.setViewportSize({ width: 390, height: 844 });

    const initialDiskUsageRead = waitForHttp(testPage, "GET", /\/api\/v1\/system\/disk-usage$/);
    await testPage.goto("/settings/system/status");
    await initialDiskUsageRead;

    await expect(testPage.getByTestId("system-disk-usage-card")).toBeVisible();
    await expect(testPage.getByTestId("system-disk-usage-table")).toBeVisible();
    await expect(testPage.getByTestId("system-disk-usage-total")).toBeVisible();
    await expect(testPage.getByTestId("system-disk-usage-computed-at")).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "System Status disk usage on mobile");

    const requestOrder: string[] = [];
    const captureDiskRequests = (request: { url: () => string; method: () => string }) => {
      if (request.url().includes("/api/v1/system/disk-usage/refresh")) {
        requestOrder.push(request.method());
      } else if (
        request.url().includes("/api/v1/system/disk-usage") &&
        request.method() === "GET"
      ) {
        requestOrder.push(request.method());
      }
    };
    testPage.on("request", captureDiskRequests);
    const refreshPost = testPage.waitForRequest(
      (request) =>
        request.url().includes("/api/v1/system/disk-usage/refresh") && request.method() === "POST",
      { timeout: 10_000 },
    );
    const refreshedUsage = testPage.waitForRequest(
      (response) =>
        response.url().includes("/api/v1/system/disk-usage") && response.method() === "GET",
      { timeout: 15_000 },
    );

    const refreshButton = testPage.getByTestId("system-disk-usage-refresh");
    await refreshButton.tap();
    await Promise.all([refreshPost, refreshedUsage]);
    await expect(refreshButton).toHaveAttribute("data-state", "success");
    expect(requestOrder.slice(0, 2)).toEqual(["POST", "GET"]);
    testPage.off("request", captureDiskRequests);
    await expect(testPage.getByTestId("system-disk-usage-table")).toBeVisible();
    await expect(testPage.getByTestId("system-disk-usage-total")).toBeVisible();
    await expect(testPage.getByTestId("system-disk-usage-computed-at")).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "System Status after disk usage refresh");
  });
});
