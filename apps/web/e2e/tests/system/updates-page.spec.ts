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
    await expect(testPage.getByTestId("system-updates-apply")).toHaveCount(0);
  });

  test("Apply update is available only for managed service installs", async ({ testPage }) => {
    test.setTimeout(30_000);

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
          install: {
            running_as_service: true,
            managed_service: true,
            mode: "user",
            manager: "systemd",
            kind: "npm",
          },
          apply_supported: true,
        }),
      });
    });

    let applyCalled = false;
    await testPage.route("**/api/v1/system/updates/apply", (route) => {
      applyCalled = true;
      void route.fulfill({
        status: 202,
        contentType: "application/json",
        body: JSON.stringify({ job_id: "self-update-1" }),
      });
    });

    await testPage.goto("/settings/system/updates");
    await testPage.getByTestId("system-updates-check").click();
    await expect(testPage.getByTestId("system-updates-apply")).toBeVisible({ timeout: 10_000 });

    await testPage.getByTestId("system-updates-apply").click();
    await testPage.getByTestId("system-updates-apply-confirm").click();
    await expect.poll(() => applyCalled).toBe(true);
  });

  test("apply locks the button and shows progress until the version flips", async ({
    testPage,
  }) => {
    test.setTimeout(30_000);

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
          install: {
            running_as_service: true,
            managed_service: true,
            mode: "user",
            manager: "launchd",
            kind: "npm",
          },
          apply_supported: true,
        }),
      });
    });
    await testPage.route("**/api/v1/system/updates/apply", (route) => {
      void route.fulfill({
        status: 202,
        contentType: "application/json",
        body: JSON.stringify({ job_id: "self-update-1" }),
      });
    });
    await testPage.route("**/api/v1/system/jobs/**", (route) => {
      void route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          id: "self-update-1",
          kind: "self-update",
          state: "succeeded",
          started_at: new Date().toISOString(),
        }),
      });
    });
    // The version poll is the "did it land" signal: report the target version so
    // the flow reaches "done".
    await testPage.route("**/api/v1/system/info", (route) => {
      void route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          version: "v1.0.1",
          commit: "abc",
          build_time: new Date().toISOString(),
          go_version: "go1.26",
          os: "darwin",
          arch: "arm64",
        }),
      });
    });

    await testPage.goto("/settings/system/updates");
    await testPage.getByTestId("system-updates-check").click();
    await testPage.getByTestId("system-updates-apply").click();
    await testPage.getByTestId("system-updates-apply-confirm").click();

    const progress = testPage.getByTestId("system-updates-progress");
    await expect(progress).toBeVisible({ timeout: 10_000 });
    // The button must not be re-triggerable while/after the update runs.
    await expect(testPage.getByTestId("system-updates-apply")).toHaveCount(0);
    await expect(progress).toHaveAttribute("data-phase", "done", { timeout: 10_000 });
    await expect(progress).toContainText("Updated to v1.0.1");
  });

  test("mobile keeps Apply update hidden for non-service installs", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
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
          install: { running_as_service: false, managed_service: false },
          apply_supported: false,
          apply_unsupported_reason: "Kandev is not running as a managed service.",
          manual_commands: ["kandev service install"],
        }),
      });
    });

    await testPage.goto("/settings/system/updates");
    await testPage.getByTestId("system-updates-check").click();

    await expect(testPage.getByTestId("system-updates-apply")).toHaveCount(0);
    await expect(testPage.getByTestId("system-updates-manual")).toBeVisible();
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
    const page2 = changelogPagination.getByTestId("changelog-page-2");
    const hasPagination = (await page2.count()) > 0;
    test.skip(!hasPagination, "Changelog has fewer than 2 pages on this build");

    await expect(changelogPagination).toBeVisible({ timeout: 15_000 });

    await page2.click();
    // URL replace should land on ?page=2 without a full reload.
    await testPage.waitForURL(/[?&]page=2(\b|&)/, { timeout: 5_000 });

    // Going back to page 1 strips the query param (clean URL convention).
    const page1 = changelogPagination.getByTestId("changelog-page-1");
    await page1.click();
    await testPage.waitForURL((url) => !url.search.includes("page="), { timeout: 5_000 });
  });
});
