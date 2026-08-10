import { test, expect } from "../../fixtures/test-base";
import { PROVIDER_NAMES, seedNotificationProviders } from "./notifications-type-scale-helpers";

test.describe("Mobile notifications settings type scale", () => {
  test("renders the stacked event list on the shared settings scale", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    await seedNotificationProviders(apiClient);

    // No setViewportSize here on purpose: this spec only runs under the
    // `mobile-chrome` project (testMatch /mobile-.*\.spec\.ts/, Pixel 5), and
    // `chromium` testIgnores the same pattern. The testPage context sets only
    // baseURL, so it inherits the project's device.
    await testPage.goto("/settings/general/notifications");

    const eventList = testPage.getByTestId("notification-events-mobile-list");
    await expect(eventList).toBeVisible();

    // One row per provider per event section, so the seeded name repeats. Match
    // them all rather than .first() — a regression confined to a later section
    // would otherwise pass.
    const providerRows = eventList.getByText(PROVIDER_NAMES[0], { exact: true });
    const rowCount = await providerRows.count();
    expect(rowCount).toBeGreaterThan(1);

    // Capture before asserting so a run against the pre-fix code still produces
    // the "before" image for the PR description.
    await prCapture.screenshot("notifications-mobile", {
      caption: "Settings → General → Notifications on a phone viewport",
    });

    // The mobile provider rows used to be text-sm while the section heading
    // above them inherited the 12px Card base — larger children under a smaller
    // heading. Every row sits at 12px now.
    const sizes = await providerRows.evaluateAll((elements) =>
      elements.map((el) => parseFloat(getComputedStyle(el).fontSize)),
    );
    expect(sizes).toHaveLength(rowCount);
    for (const size of sizes) expect(size).toBeCloseTo(12, 1);
  });
});
