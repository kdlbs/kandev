import { test, expect } from "../../fixtures/test-base";
import { APPRISE_PROVIDER_NAME, routeAppriseRescans } from "./apprise-rescan-helpers";
import { waitForHttp } from "../../helpers/causal-waits";

test.describe("Apprise rescan", () => {
  test("refreshes availability, preserves an event draft, and supports retry", async ({
    testPage,
    prCapture,
  }) => {
    const { startRescan } = await routeAppriseRescans(testPage, [true, "error", false]);
    const providersLoaded = waitForHttp(testPage, "GET", /\/api\/v1\/notification-providers$/);
    await testPage.goto("/settings/preferences/notifications");
    await providersLoaded;

    const rescan = testPage.getByTestId("apprise-rescan");
    const event = testPage.getByRole("checkbox", {
      name: `Agent turn finished for ${APPRISE_PROVIDER_NAME}`,
    });
    await expect(testPage.getByTestId("notification-events-desktop-table")).toBeVisible();
    await expect(rescan).toBeVisible();
    await expect(testPage.getByRole("button", { name: "Add Apprise Provider" })).toHaveCount(0);
    await expect(event).not.toBeChecked();

    await event.click();
    await expect(event).toBeChecked();
    await expect(testPage.getByRole("button", { name: "Save changes" })).toBeVisible();

    startRescan();
    const detected = waitForHttp(testPage, "GET", /\/api\/v1\/notification-providers$/);
    await rescan.click();
    await detected;
    await expect(testPage.getByText("Apprise detected.", { exact: true })).toBeVisible();
    await expect(testPage.getByRole("button", { name: "Add Apprise Provider" })).toBeVisible();
    await expect(event).toBeChecked();
    await expect(testPage.getByRole("button", { name: "Save changes" })).toBeVisible();
    await prCapture.screenshot("apprise-rescan-desktop", {
      caption: "Desktop Apprise detection and preserved notification draft",
    });

    const failed = waitForHttp(testPage, "GET", /\/api\/v1\/notification-providers$/);
    await rescan.click();
    await failed;
    await expect(testPage.getByTestId("apprise-rescan-error")).toHaveText(
      "Could not check Apprise. Try again.",
    );
    await expect(testPage.getByRole("button", { name: "Add Apprise Provider" })).toBeVisible();

    const retried = waitForHttp(testPage, "GET", /\/api\/v1\/notification-providers$/);
    await rescan.click();
    await retried;
    await expect(testPage.getByText("Apprise was not detected.", { exact: true })).toBeVisible();
    await expect(testPage.getByRole("button", { name: "Add Apprise Provider" })).toHaveCount(0);
    await expect(event).toBeChecked();
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
    ).toBe(false);
  });
});
