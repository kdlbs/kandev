import { test, expect } from "../../fixtures/test-base";
import {
  assertNoDescendantOverflowsRight,
  assertNoDocumentHorizontalOverflow,
} from "../../helpers/layout-assertions";
import {
  PROVIDER_NAMES,
  expectNormalPermissionLayout,
  expectPermissionWarning,
  expectPhoneTouchTargets,
  expectPreferenceComposition,
  notificationPermissionGroup,
  notificationSoundGroup,
  requirePreferenceBox,
  seedNotificationProviders,
  setStubbedNotificationPermission,
  stubNotificationPermission,
} from "./notifications-type-scale-helpers";

const SAVE_LABEL = "Save changes";

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

  test("renders compact phone rows with touch targets and a preserved sound draft", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    await seedNotificationProviders(apiClient);
    await stubNotificationPermission(testPage, "default");
    await testPage.goto("/settings/general/notifications");

    await expect(
      testPage.getByRole("heading", { level: 2, name: "Notifications", exact: true }),
    ).toHaveCount(1);
    await expect(
      testPage.getByText(
        "Choose how this device alerts you to agent turns, questions, and Office events.",
      ),
    ).toBeVisible();

    const permission = notificationPermissionGroup(testPage);
    const sound = notificationSoundGroup(testPage);
    await expect(permission.warning).toHaveCount(0);
    await expect(permission.footer).toHaveCount(0);
    await expectPreferenceComposition(testPage, "phone");
    await expectPhoneTouchTargets(testPage);

    // On phones the selector fills the remaining control row beside the
    // preview, rather than keeping its fixed desktop width.
    const controlsBox = requirePreferenceBox(
      await sound.controls.boundingBox(),
      "phone sound controls",
    );
    const selectBox = requirePreferenceBox(await sound.select.boundingBox(), "phone sound preset");
    const previewBox = requirePreferenceBox(
      await sound.preview.boundingBox(),
      "phone sound preview",
    );
    expect(Math.abs(selectBox.x - controlsBox.x)).toBeLessThanOrEqual(1);
    expect(
      Math.abs(previewBox.x + previewBox.width - (controlsBox.x + controlsBox.width)),
    ).toBeLessThanOrEqual(1);
    const controlGap = previewBox.x - (selectBox.x + selectBox.width);
    expect(controlGap).toBeGreaterThanOrEqual(7);
    expect(controlGap).toBeLessThanOrEqual(9);
    await assertNoDescendantOverflowsRight(sound.group, "phone sound row");
    await assertNoDocumentHorizontalOverflow(testPage, "phone notification preferences");

    await sound.toggle.tap();
    await expect(sound.toggle).toHaveAttribute("aria-checked", "true");
    await sound.select.tap();
    await testPage.getByRole("option", { name: "Ding", exact: true }).tap();
    await expect(sound.select).toContainText("Ding");
    await sound.preview.tap();
    await prCapture.screenshot("notifications-sound-enabled-mobile", {
      caption: "Phone notification sound enabled with two control rows",
    });

    // The draft survives the desktop composition on the way to Save.
    await testPage.setViewportSize({ width: 1280, height: 1400 });
    await expectPreferenceComposition(testPage, "desktop");
    await expect(sound.toggle).toHaveAttribute("aria-checked", "true");
    await expect(sound.select).toContainText("Ding");

    await testPage.setViewportSize({ width: 393, height: 851 });
    await expectPreferenceComposition(testPage, "phone");
    await testPage.getByRole("button", { name: SAVE_LABEL, exact: true }).click();
    await expect(testPage.getByRole("button", { name: SAVE_LABEL, exact: true })).toHaveCount(0);

    await testPage.reload();
    const reloaded = notificationSoundGroup(testPage);
    await expect(reloaded.toggle).toHaveAttribute("aria-checked", "true");
    await expect(reloaded.select).toContainText("Ding");

    // Reset restores the saved switch state and preset instead of a default.
    await reloaded.toggle.tap();
    await expect(reloaded.toggle).toHaveAttribute("aria-checked", "false");
    await expect(reloaded.select).toBeDisabled();
    await testPage.getByRole("button", { name: "Reset", exact: true }).tap();
    await expect(reloaded.toggle).toHaveAttribute("aria-checked", "true");
    await expect(reloaded.select).toContainText("Ding");
    await expect(reloaded.select).toBeEnabled();
    await assertNoDocumentHorizontalOverflow(testPage, "phone reset notification preferences");
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

    const { group, refresh, label } = notificationPermissionGroup(testPage);

    // Capture before asserting so a run against the pre-fix code still produces
    // the "before" image for the PR description.
    await prCapture.screenshot("notifications-permission-default-mobile", {
      caption: "Desktop notifications on a phone with undecided permission and no warning row",
    });
    await expectNormalPermissionLayout(testPage);
    await expectPreferenceComposition(testPage, "phone");

    // Below md the actions move under the label instead of sharing its row.
    const labelBox = requirePreferenceBox(await label.boundingBox(), "phone permission label");
    const refreshBox = requirePreferenceBox(
      await refresh.boundingBox(),
      "phone permission refresh",
    );
    expect(refreshBox.y).toBeGreaterThanOrEqual(labelBox.y + labelBox.height - 1);

    await setStubbedNotificationPermission(testPage, "denied");
    await refresh.tap();

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
      caption: "Desktop notifications on a phone with browser-denied permission feedback",
    });

    await setStubbedNotificationPermission(testPage, "granted");
    await refresh.tap();

    await expect(group.getByRole("button", { name: "Enabled" })).toBeDisabled();
    await expectNormalPermissionLayout(testPage);
    await assertNoDocumentHorizontalOverflow(testPage, "phone granted permission");
  });
});
