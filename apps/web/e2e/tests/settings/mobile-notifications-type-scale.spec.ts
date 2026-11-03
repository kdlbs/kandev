import { test, expect } from "../../fixtures/test-base";
import {
  assertNoDescendantOverflowsRight,
  assertNoDocumentHorizontalOverflow,
} from "../../helpers/layout-assertions";
import {
  PROVIDER_NAMES,
  expectNormalPermissionLayout,
  expectPermissionWarning,
  notificationPermissionGroup,
  seedNotificationProviders,
  setStubbedNotificationPermission,
  stubNotificationPermission,
} from "./notifications-type-scale-helpers";

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

  test("permission feedback has no empty frame and follows refresh", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    await seedNotificationProviders(apiClient);
    await stubNotificationPermission(testPage, "default");

    // No setViewportSize here on purpose: this spec only runs under the
    // `mobile-chrome` project (Pixel 5), like the type-scale case above.
    await testPage.goto("/settings/general/notifications");

    const { group, refresh } = notificationPermissionGroup(testPage);

    // Capture before asserting so a run against the pre-fix code still produces
    // the "before" image for the PR description.
    await prCapture.screenshot("notifications-permission-default-mobile", {
      caption: "Desktop Notifications on a phone with undecided permission and no warning frame",
    });
    await expectNormalPermissionLayout(testPage);

    // Below md the header stacks: the actions move under the heading and
    // description instead of sharing their row.
    const heading = group.getByRole("heading", { name: "Desktop Notifications" });
    const headingBox = await heading.boundingBox();
    const actionsBox = await refresh.boundingBox();
    expect(headingBox).not.toBeNull();
    expect(actionsBox).not.toBeNull();
    expect(actionsBox!.y).toBeGreaterThanOrEqual(headingBox!.y + headingBox!.height - 1);

    await setStubbedNotificationPermission(testPage, "denied");
    await refresh.click();

    await expectPermissionWarning(testPage, /site settings/i);
    await expect(group.getByRole("button", { name: "Enable" })).toBeVisible();
    await assertNoDescendantOverflowsRight(group, "phone permission warning");
    await assertNoDocumentHorizontalOverflow(testPage, "phone permission warning");

    // The settings content column stays the one page scroll owner; the warning
    // itself must not introduce a nested scroller.
    await expect(testPage.getByTestId("settings-scroll-container")).toHaveCSS("overflow-y", "auto");
    const closestScrollOwner = await group
      .getByTestId("notification-permission-warning")
      .evaluate((element) => {
        for (let node = element.parentElement; node; node = node.parentElement) {
          const overflowY = getComputedStyle(node).overflowY;
          if (overflowY === "auto" || overflowY === "scroll") {
            return node.getAttribute("data-testid") ?? node.tagName.toLowerCase();
          }
        }
        return null;
      });
    expect(closestScrollOwner).toBe("settings-scroll-container");

    await prCapture.screenshot("notifications-permission-denied-mobile", {
      caption: "Desktop Notifications on a phone with browser-denied permission feedback",
    });

    await setStubbedNotificationPermission(testPage, "granted");
    await refresh.click();

    await expect(group.getByRole("button", { name: "Enabled" })).toBeDisabled();
    await expectNormalPermissionLayout(testPage);
    await assertNoDocumentHorizontalOverflow(testPage, "phone granted permission");
  });
});
