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
});
