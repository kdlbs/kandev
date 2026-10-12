import { test, expect } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";

// Wait until the backend has a populated disk-usage cache (the initial walk
// is async; the first call returns `{data: null, computing: true}` and kicks
// it off, subsequent calls return the cached breakdown). We poll the API
// before loading the page so the client's first fetch lands on populated
// data without depending on a WS event that may race against page mount.
async function waitForDiskUsageCached(apiClient: {
  rawRequest: (m: string, p: string) => Promise<Response>;
}): Promise<void> {
  const deadline = Date.now() + 20_000;
  while (Date.now() < deadline) {
    const res = await apiClient.rawRequest("GET", "/api/v1/system/disk-usage");
    if (res.ok) {
      const body = (await res.json()) as { data: unknown; computing: boolean };
      if (body.data) return;
    }
    await dwell(
      250,
      "poll-interval",
      "sampling interval for the disk-usage cache poll above; the cache is computed in the background and only observable by re-reading the endpoint",
    );
  }
  throw new Error("disk-usage cache never populated within 20s");
}

test.describe("System Status — disk usage", () => {
  test("disk usage breakdown renders once the cache is populated", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);

    await waitForDiskUsageCached(apiClient);

    await testPage.goto("/settings/system/status");

    const card = testPage.getByTestId("system-disk-usage-card");
    await expect(card).toBeVisible();

    await expect(testPage.getByTestId("system-disk-usage-table")).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("system-disk-usage-total")).toBeVisible();
  });

  test("refresh button hits the refresh endpoint and re-fetches usage", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);

    await waitForDiskUsageCached(apiClient);

    await testPage.goto("/settings/system/status");
    await expect(testPage.getByTestId("system-disk-usage-table")).toBeVisible({ timeout: 15_000 });

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
    const refreshCalled = testPage.waitForRequest(
      (req) => req.url().includes("/api/v1/system/disk-usage/refresh") && req.method() === "POST",
      { timeout: 10_000 },
    );
    const refreshedUsage = testPage.waitForRequest(
      (response) =>
        response.url().includes("/api/v1/system/disk-usage") && response.method() === "GET",
      { timeout: 15_000 },
    );

    await testPage.getByTestId("system-disk-usage-refresh").click();
    await Promise.all([refreshCalled, refreshedUsage]);
    await expect(testPage.getByTestId("system-disk-usage-refresh")).toHaveAttribute(
      "data-state",
      "success",
    );
    expect(requestOrder.slice(0, 2)).toEqual(["POST", "GET"]);
    testPage.off("request", captureDiskRequests);
    await expect(testPage.getByTestId("system-disk-usage-table")).toBeVisible();
    await expect(testPage.getByTestId("system-disk-usage-total")).toBeVisible();
    await expect(testPage.getByTestId("system-disk-usage-computed-at")).toBeVisible();
  });

  test("shows a handled refresh POST error on the existing card and keeps its snapshot", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);
    await waitForDiskUsageCached(apiClient);
    await testPage.goto("/settings/system/status");
    await expect(testPage.getByTestId("system-disk-usage-table")).toBeVisible({ timeout: 15_000 });

    await testPage.route("**/api/v1/system/disk-usage/refresh", (route) =>
      route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: "Disk refresh is unavailable" }),
      }),
    );
    await testPage.getByTestId("system-disk-usage-refresh").click();

    await expect(testPage.getByTestId("system-disk-usage-error")).toHaveText(
      "Disk refresh is unavailable",
    );
    await expect(testPage.getByTestId("system-disk-usage-table")).toBeVisible();
    await expect(testPage.getByTestId("system-disk-usage-total")).toBeVisible();
  });

  test("data directory path and Open button are exposed on the card", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);
    await waitForDiskUsageCached(apiClient);

    await testPage.goto("/settings/system/status");

    const homeDirBlock = testPage.getByTestId("system-disk-usage-home-dir");
    await expect(homeDirBlock).toBeVisible({ timeout: 15_000 });
    // The path is rendered in font-mono inside the block; just assert it is
    // non-empty (the exact path varies by env / config).
    const text = await homeDirBlock.innerText();
    expect(text.trim().length).toBeGreaterThan("DATA DIRECTORY".length);

    const openRequest = testPage.waitForRequest(
      (req) => req.url().includes("/api/v1/system/disk-usage/open") && req.method() === "POST",
      { timeout: 5_000 },
    );
    await testPage.getByTestId("system-disk-usage-open").click();
    await openRequest;
  });

  test("page first-loads without manual refresh even when WS event is missed", async ({
    testPage,
  }) => {
    // Regression: the disk-walk job's completion is published over WS; if the
    // socket isn't connected at the moment the walk finishes the broadcast is
    // dropped and the card used to sit on "Calculating..." forever. The hook
    // now polls every ~1.5s while the backend reports computing=true, so the
    // table must appear within a few seconds on a fresh page load — no
    // intermediate apiClient pre-warm here, on purpose.
    test.setTimeout(30_000);

    await testPage.goto("/settings/system/status");
    await expect(testPage.getByTestId("system-disk-usage-card")).toBeVisible();
    await expect(testPage.getByTestId("system-disk-usage-table")).toBeVisible({
      timeout: 20_000,
    });
  });
});
