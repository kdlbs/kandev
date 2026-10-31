import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  expectNormalPermissionLayout,
  expectPermissionWarning,
  notificationPermissionGroup,
  seedNotificationProviders,
  setStubbedNotificationPermission,
  stubNotificationPermission,
} from "./notifications-type-scale-helpers";

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

    // Group descriptions use the readable section role (14px), while the
    // table body remains compact (12px). Assert the computed sizes so the
    // capture is backed by a real measurement, not just an eyeball.
    expect(
      await description.evaluate((el) => parseFloat(getComputedStyle(el).fontSize)),
    ).toBeCloseTo(14, 1);
    expect(await table.evaluate((el) => parseFloat(getComputedStyle(el).fontSize))).toBeCloseTo(
      12,
      1,
    );
  });

  test("permission feedback has no empty frame and follows refresh", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    await seedNotificationProviders(apiClient);
    await stubNotificationPermission(testPage, "default");

    await testPage.setViewportSize({ width: 1280, height: 1100 });
    await testPage.goto("/settings/general/notifications");

    const { group, refresh } = notificationPermissionGroup(testPage);

    // Capture before asserting so a run against the pre-fix code still produces
    // the "before" image for the PR description.
    await prCapture.screenshot("notifications-permission-default", {
      caption: "Desktop Notifications with undecided permission and no warning frame",
    });
    await expectNormalPermissionLayout(testPage);

    await setStubbedNotificationPermission(testPage, "denied");
    await refresh.click();

    await expectPermissionWarning(testPage, /site settings/i);
    await expect(group.getByRole("button", { name: "Enable" })).toBeEnabled();
    await assertNoDocumentHorizontalOverflow(testPage, "desktop permission warning");

    await prCapture.screenshot("notifications-permission-denied", {
      caption: "Desktop Notifications with browser-denied permission feedback",
    });

    await setStubbedNotificationPermission(testPage, "granted");
    await refresh.click();

    // The enabled label is the positive signal that Refresh re-read permission;
    // asserting the absent card first would pass before the state settled.
    await expect(group.getByRole("button", { name: "Enabled" })).toBeDisabled();
    await expectNormalPermissionLayout(testPage);
    await assertNoDocumentHorizontalOverflow(testPage, "desktop granted permission");
  });
});
