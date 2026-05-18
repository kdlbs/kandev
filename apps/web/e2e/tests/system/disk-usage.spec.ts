import { test, expect } from "../../fixtures/test-base";

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
    await new Promise((r) => setTimeout(r, 250));
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

    const refreshCalled = testPage.waitForRequest(
      (req) => req.url().includes("/api/v1/system/disk-usage/refresh") && req.method() === "POST",
      { timeout: 10_000 },
    );

    await testPage.getByTestId("system-disk-usage-refresh").click();
    await refreshCalled;
  });
});
