import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
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

  test("shows one introduction and compact device rows before push", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    await seedNotificationProviders(apiClient);
    await stubNotificationPermission(testPage, "default");
    await testPage.setViewportSize({ width: 1280, height: 1400 });
    await testPage.goto("/settings/general/notifications");

    await expect(
      testPage.getByRole("heading", { level: 2, name: "Notifications", exact: true }),
    ).toHaveCount(1);
    await expect(
      testPage.getByText(
        "Choose how this device alerts you to agent turns, questions, and Office events.",
      ),
    ).toBeVisible();
    await expect(
      testPage.getByText(
        "Notify this device when a selected agent turn, question, or Office event occurs.",
      ),
    ).toHaveCount(0);
    await expect(
      testPage.getByText(
        "Play a sound on this device when a selected agent turn, question, or Office event occurs.",
      ),
    ).toHaveCount(0);
    await expect(
      testPage.getByRole("heading", { name: "Desktop Notifications", exact: true }),
    ).toHaveCount(0);
    await expect(
      testPage.getByRole("heading", { name: "Notification Sound", exact: true }),
    ).toHaveCount(0);

    const permission = notificationPermissionGroup(testPage);
    const sound = notificationSoundGroup(testPage);
    await expect(permission.group).toBeVisible();
    await expect(sound.group).toBeVisible();
    await expect(permission.warning).toHaveCount(0);
    await expect(permission.footer).toHaveCount(0);
    await expect(sound.select).toBeVisible();
    await expect(sound.preview).toBeVisible();
    await expect(sound.select).toBeDisabled();
    await expect(sound.preview).toBeDisabled();

    await expectPreferenceComposition(testPage, "desktop");

    const soundBox = requirePreferenceBox(await sound.group.boundingBox(), "sound row");
    const pushBox = requirePreferenceBox(
      await testPage.getByRole("heading", { name: "Push Notifications" }).boundingBox(),
      "push heading",
    );
    expect(soundBox.y).toBeLessThan(pushBox.y);
    await assertNoDocumentHorizontalOverflow(testPage, "desktop compact device rows");

    await prCapture.screenshot("notifications-compact-device-rows", {
      caption: "One introduction and compact device rows before Push notifications",
    });
  });

  test("keeps stable sound geometry and retains a draft across the 767/768 boundary", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    await seedNotificationProviders(apiClient);
    await stubNotificationPermission(testPage, "default");
    await testPage.setViewportSize({ width: 1280, height: 1400 });
    await testPage.goto("/settings/general/notifications");

    const sound = notificationSoundGroup(testPage);
    await expect(sound.group).toBeVisible();
    await expect(sound.toggle).toHaveAttribute("aria-checked", "false");

    const geometry = async () => {
      const row = requirePreferenceBox(await sound.group.boundingBox(), "sound row");
      const select = requirePreferenceBox(await sound.select.boundingBox(), "sound preset");
      const preview = requirePreferenceBox(await sound.preview.boundingBox(), "sound preview");
      return { rowHeight: row.height, selectY: select.y, previewY: preview.y };
    };

    await sound.toggle.click();
    await expect(sound.toggle).toHaveAttribute("aria-checked", "true");
    await sound.select.click();
    await testPage.getByRole("option", { name: "Ding", exact: true }).click();
    await expect(sound.select).toContainText("Ding");
    await sound.preview.click();
    const enabledGeometry = await geometry();
    await prCapture.screenshot("notifications-sound-enabled", {
      caption: "Desktop notification sound enabled with preset and preview",
    });

    await sound.toggle.click();
    await expect(sound.toggle).toHaveAttribute("aria-checked", "false");
    await expect(sound.select).toBeDisabled();
    await expect(sound.preview).toBeDisabled();
    await expect(sound.select).toContainText("Ding");
    const disabledGeometry = await geometry();
    expect(Math.abs(enabledGeometry.rowHeight - disabledGeometry.rowHeight)).toBeLessThanOrEqual(1);
    expect(Math.abs(enabledGeometry.selectY - disabledGeometry.selectY)).toBeLessThanOrEqual(1);
    expect(Math.abs(enabledGeometry.previewY - disabledGeometry.previewY)).toBeLessThanOrEqual(1);
    await prCapture.screenshot("notifications-sound-disabled", {
      caption: "Desktop notification sound disabled with stable preset and preview controls",
    });

    await sound.toggle.click();
    await expect(sound.toggle).toHaveAttribute("aria-checked", "true");

    await testPage.setViewportSize({ width: 767, height: 1200 });
    await expectPreferenceComposition(testPage, "phone");
    await expectPhoneTouchTargets(testPage);
    await assertNoDocumentHorizontalOverflow(testPage, "767px notification preferences");

    await testPage.setViewportSize({ width: 768, height: 1200 });
    await expectPreferenceComposition(testPage, "desktop");
    await assertNoDocumentHorizontalOverflow(testPage, "768px notification preferences");

    await testPage.setViewportSize({ width: 1280, height: 1400 });
    await expect(sound.toggle).toHaveAttribute("aria-checked", "true");
    await expect(sound.select).toContainText("Ding");
    await expect(sound.preview).toBeEnabled();

    await testPage.getByRole("button", { name: SAVE_LABEL, exact: true }).click();
    await expect(testPage.getByRole("button", { name: SAVE_LABEL, exact: true })).toHaveCount(0);

    await testPage.reload();
    const reloaded = notificationSoundGroup(testPage);
    await expect(reloaded.toggle).toHaveAttribute("aria-checked", "true");
    await expect(reloaded.select).toContainText("Ding");
    await expect(reloaded.preview).toBeEnabled();
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

    const { group, refresh, enable } = notificationPermissionGroup(testPage);

    // Capture before asserting so a run against the pre-fix code still produces
    // the "before" image for the PR description.
    await prCapture.screenshot("notifications-permission-default", {
      caption: "Desktop notifications with undecided permission and no warning row",
    });
    await expectNormalPermissionLayout(testPage);
    await expectPreferenceComposition(testPage, "desktop");

    await setStubbedNotificationPermission(testPage, "denied");
    await refresh.click();

    await expectPermissionWarning(testPage, /site settings/i);
    await expect(enable).toBeEnabled();
    await assertNoDocumentHorizontalOverflow(testPage, "desktop permission warning");

    await prCapture.screenshot("notifications-permission-denied", {
      caption: "Desktop notifications with browser-denied permission feedback",
    });

    await setStubbedNotificationPermission(testPage, "granted");
    await refresh.click();

    // The enabled label is the positive signal that Refresh re-read permission;
    // asserting the absent row first would pass before the state settled.
    await expect(group.getByRole("button", { name: "Enabled" })).toBeDisabled();
    await expectNormalPermissionLayout(testPage);
    await assertNoDocumentHorizontalOverflow(testPage, "desktop granted permission");
  });
});
