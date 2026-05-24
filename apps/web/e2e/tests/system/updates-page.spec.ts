import { test, expect } from "../../fixtures/test-base";

test.describe("System Updates page", () => {
  test("clicking Check now with a stubbed 'update available' response shows the badge and versions", async ({
    testPage,
  }) => {
    test.setTimeout(30_000);

    // The /updates GET fires server-side during SSR (can't be intercepted by
    // Playwright's testPage.route), so we drive the state via the client-only
    // /updates/check POST instead — the hook writes its response into the
    // store and the card re-renders with the stubbed values.
    await testPage.route("**/api/v1/system/updates/check", (route) => {
      void route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          current: "v1.0.0",
          latest: "v1.0.1",
          latest_url: "https://example.com/r/v1.0.1",
          latest_checked_at: new Date().toISOString(),
          update_available: true,
        }),
      });
    });

    await testPage.goto("/settings/system/updates");
    await expect(testPage.getByTestId("system-page-title")).toHaveText("Updates");

    await testPage.getByTestId("system-updates-check").click();

    const badge = testPage.getByTestId("system-updates-badge");
    await expect(badge).toBeVisible({ timeout: 10_000 });
    await expect(badge).toHaveText(/update available/i);

    await expect(testPage.getByTestId("system-updates-current")).toHaveText("v1.0.0");
    await expect(testPage.getByTestId("system-updates-latest")).toHaveText("v1.0.1");
  });

  test("changelog pagination is URL-driven via ?page=N", async ({ testPage }) => {
    // The changelog renders 10 entries per page and the embedded list (built
    // from generated/changelog.json) routinely covers >10 versions in this
    // repo, so page 2 should exist. If a future trim drops it below 11 the
    // pagination element would not render and the test would skip; protect
    // against that by asserting on the page-2 link only when present.
    await testPage.goto("/settings/system/updates");
    await expect(testPage.getByTestId("system-page-title")).toHaveText("Updates");

    // Scope to changelog pagination — settings sidebar workspace links can also
    // expose accessible names that match single-digit page numbers.
    const changelogPagination = testPage.getByTestId("changelog-pagination");
    await expect(changelogPagination).toBeVisible({ timeout: 15_000 });
    const page2 = changelogPagination.getByTestId("changelog-page-2");
    const hasPagination = (await page2.count()) > 0;
    test.skip(!hasPagination, "Changelog has fewer than 2 pages on this build");

    await page2.click();
    // URL replace should land on ?page=2 without a full reload.
    await testPage.waitForURL(/[?&]page=2(\b|&)/, { timeout: 5_000 });

    // Going back to page 1 strips the query param (clean URL convention).
    const page1 = changelogPagination.getByTestId("changelog-page-1");
    await page1.click();
    await testPage.waitForURL((url) => !url.search.includes("page="), { timeout: 5_000 });
  });
});
