import { test, expect } from "../../fixtures/test-base";
import { seedNotificationProviders } from "./notifications-type-scale-helpers";

test.describe("Notifications settings type scale", () => {
  test("renders the card body on the shared settings scale", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    await seedNotificationProviders(apiClient);

    await testPage.setViewportSize({ width: 1280, height: 1100 });
    await testPage.goto("/settings/general/notifications");

    const table = testPage.getByTestId("notification-events-desktop-table");
    await expect(table).toBeVisible();
    const description = testPage.getByTestId("external-providers-description");
    await expect(description).toBeVisible();

    // Capture before asserting so a run against the pre-fix code still produces
    // the "before" image for the PR description.
    await prCapture.screenshot("notifications-desktop", {
      caption: "Settings → General → Notifications at 1280px",
    });

    // The page body inherits the Card base (text-xs/relaxed = 12px); group
    // headings sit one step up at text-sm (14px). Assert the computed sizes so
    // the capture is backed by a real measurement, not just an eyeball.
    expect(
      await description.evaluate((el) => parseFloat(getComputedStyle(el).fontSize)),
    ).toBeCloseTo(12, 1);
    expect(await table.evaluate((el) => parseFloat(getComputedStyle(el).fontSize))).toBeCloseTo(
      12,
      1,
    );
  });
});
