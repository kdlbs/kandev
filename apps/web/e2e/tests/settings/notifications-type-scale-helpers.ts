import { expect } from "../../fixtures/test-base";
import type { Locator, Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";

const TURN_FINISHED = "session.turn_finished";
const CLARIFICATION_REQUESTED = "session.clarification_requested";

/** Two providers so the desktop events table renders more than one column. */
export const PROVIDER_NAMES = ["Desktop", "Team channel"];

/** Permission values the settings page can read from `Notification.permission`. */
export type StubbedNotificationPermission = "default" | "denied" | "granted";

export type PreferenceComposition = "desktop" | "phone";

const PERMISSION_STUB_SETTER = "__kandevSetNotificationPermission";

/**
 * The settings type scale is shared by the desktop table and the mobile
 * stacked list, so both specs seed the same providers. Keeping one helper stops
 * the two from drifting into asserting against different fixtures.
 */
export async function seedNotificationProviders(apiClient: ApiClient): Promise<void> {
  for (const name of PROVIDER_NAMES) {
    const response = await apiClient.rawRequest("POST", "/api/v1/notification-providers", {
      name,
      type: "local",
      events: [TURN_FINISHED, CLARIFICATION_REQUESTED],
    });
    expect(response.ok).toBe(true);
  }
}

/**
 * Replaces only the read-only `Notification.permission` getter so the settings
 * page renders a deterministic permission state. The notification constructor
 * and `requestPermission` stay intact, and nothing is granted at the browser
 * level, so no operating-system prompt can open.
 */
export async function stubNotificationPermission(
  page: Page,
  initial: StubbedNotificationPermission,
): Promise<void> {
  await page.addInitScript(
    ({ initialPermission, setterName }) => {
      const NativeNotification = window.Notification;
      if (!NativeNotification) return;
      let permission: NotificationPermission = initialPermission;
      Object.defineProperty(NativeNotification, "permission", {
        configurable: true,
        get: () => permission,
      });
      Object.defineProperty(window, setterName, {
        configurable: true,
        value: (next: NotificationPermission) => {
          permission = next;
        },
      });
    },
    { initialPermission: initial, setterName: PERMISSION_STUB_SETTER },
  );
}

/**
 * Moves the stubbed permission without a browser prompt. The settings page only
 * re-reads permission through its Refresh action, so callers click Refresh
 * afterwards to observe the added or removed warning surface.
 */
export async function setStubbedNotificationPermission(
  page: Page,
  permission: StubbedNotificationPermission,
): Promise<void> {
  await page.evaluate(
    ({ setterName, next }) => {
      const setter = (window as unknown as Record<string, unknown>)[setterName];
      if (typeof setter !== "function") {
        throw new Error("notification permission stub is not installed");
      }
      (setter as (value: string) => void)(next);
    },
    { setterName: PERMISSION_STUB_SETTER, next: permission },
  );
}

/** One locator bundle so desktop and phone specs assert the same surfaces. */
export function notificationPermissionGroup(page: Page) {
  const group = page.getByTestId("notification-permission-group");
  return {
    group,
    row: group,
    footer: group.locator('[data-notification-preference-footer="true"]'),
    warning: group.getByTestId("notification-permission-warning"),
    label: group.getByText("Desktop Notifications", { exact: true }),
    enable: group.getByRole("button", { name: "Enable", exact: true }),
    refresh: group.getByRole("button", { name: "Refresh notification permission" }),
    test: group.getByRole("button", { name: "Send test notification" }),
  };
}

/** The sound row keeps every control mounted in both switch states. */
export function notificationSoundGroup(page: Page) {
  const group = page.getByTestId("notification-sound-group");
  return {
    group,
    row: group,
    controls: group.locator('[data-notification-preference-controls="true"]'),
    label: group.getByText("Notification Sound", { exact: true }),
    toggle: group.getByRole("switch", { name: "Enable notification sound" }),
    toggleTarget: group.getByTestId("notification-sound-toggle-target"),
    select: group.getByRole("combobox", { name: "Notification sound" }),
    preview: group.getByRole("button", { name: "Preview sound" }),
  };
}

export function requirePreferenceBox(
  box: Awaited<ReturnType<Locator["boundingBox"]>>,
  label: string,
) {
  expect(box, `${label}: locator has no bounding box`).not.toBeNull();
  if (!box) throw new Error(`${label}: locator has no bounding box`);
  return box;
}

const requireBox = requirePreferenceBox;

/** Two controls share a row when their overlap covers most of the smaller box. */
export async function expectSameRow(first: Locator, second: Locator, label: string): Promise<void> {
  const firstBox = requireBox(await first.boundingBox(), `${label}: first`);
  const secondBox = requireBox(await second.boundingBox(), `${label}: second`);
  const overlap = Math.max(
    0,
    Math.min(firstBox.y + firstBox.height, secondBox.y + secondBox.height) -
      Math.max(firstBox.y, secondBox.y),
  );
  const smallerHeight = Math.min(firstBox.height, secondBox.height);
  expect(
    overlap,
    `${label}: expected overlapping rows, got ${overlap.toFixed(1)}px overlap for a ${smallerHeight.toFixed(1)}px control`,
  ).toBeGreaterThanOrEqual(smallerHeight * 0.5);
}

/** The lower control starts on a later row than the upper control. */
export async function expectBelow(upper: Locator, lower: Locator, label: string): Promise<void> {
  const upperBox = requireBox(await upper.boundingBox(), `${label}: upper`);
  const lowerBox = requireBox(await lower.boundingBox(), `${label}: lower`);
  expect(
    lowerBox.y,
    `${label}: expected the lower control to start below ${(upperBox.y + upperBox.height).toFixed(
      1,
    )}`,
  ).toBeGreaterThanOrEqual(upperBox.y + upperBox.height - 1);
}

/** Asserts the desktop or phone composition of both device-preference rows. */
export async function expectPreferenceComposition(
  page: Page,
  composition: PreferenceComposition,
): Promise<void> {
  const permission = notificationPermissionGroup(page);
  const sound = notificationSoundGroup(page);

  if (composition === "desktop") {
    await expectSameRow(permission.label, permission.enable, "desktop permission row");
    await expectSameRow(sound.label, sound.toggleTarget, "desktop sound label and switch");
    await expectSameRow(sound.label, sound.select, "desktop sound label and preset");
    await expectSameRow(sound.label, sound.preview, "desktop sound label and preview");
    return;
  }

  await expectBelow(permission.label, permission.enable, "phone permission row");
  await expectSameRow(sound.label, sound.toggleTarget, "phone sound label and switch");
  await expectBelow(sound.toggleTarget, sound.select, "phone sound preset row");
  await expectSameRow(sound.select, sound.preview, "phone sound preset and preview");
}

/** Asserts 44px phone hit targets for every device-preference action. */
export async function expectPhoneTouchTargets(page: Page): Promise<void> {
  const permission = notificationPermissionGroup(page);
  const sound = notificationSoundGroup(page);
  for (const control of [
    permission.enable,
    permission.refresh,
    permission.test,
    sound.toggleTarget,
    sound.select,
    sound.preview,
  ]) {
    const box = requireBox(await control.boundingBox(), "phone touch target");
    expect(box.height, "phone touch target height").toBeGreaterThanOrEqual(44);
    expect(box.width, "phone touch target width").toBeGreaterThanOrEqual(44);
  }
}

/** Asserts the normal composition has no warning element or reserved footer. */
export async function expectNormalPermissionLayout(page: Page): Promise<void> {
  const { group, row, footer, warning } = notificationPermissionGroup(page);
  await expect(group).toBeVisible();
  await expect(row).toBeVisible();
  await expect(warning).toHaveCount(0);
  // A warning footer would reserve a second body row; normal states have none.
  await expect(footer).toHaveCount(0);
  await expect(group.locator('[data-settings-group-card="true"]')).toHaveCount(0);

  // Measured bounds keep a blank body area from passing the count assertions:
  // the compact row is body + its own vertical padding, followed by the 12px
  // preference-row gap.
  const geometry = await group.evaluate((element) => {
    const style = getComputedStyle(element);
    const body = element.querySelector('[data-notification-preference-body="true"]');
    const next = element.nextElementSibling;
    const rowBox = element.getBoundingClientRect();
    return {
      paddingTop: parseFloat(style.paddingTop),
      paddingBottom: parseFloat(style.paddingBottom),
      clientHeight: element.clientHeight,
      bodyHeight: body?.getBoundingClientRect().height ?? null,
      nextGap: next ? next.getBoundingClientRect().top - rowBox.bottom : null,
    };
  });

  expect(geometry.bodyHeight).not.toBeNull();
  if (geometry.bodyHeight === null) throw new Error("normal permission row has no body");
  expect(geometry.paddingTop).toBeGreaterThanOrEqual(10);
  expect(geometry.paddingTop).toBeLessThanOrEqual(12);
  expect(geometry.paddingBottom).toEqual(geometry.paddingTop);
  expect(
    Math.abs(
      geometry.clientHeight - geometry.bodyHeight - geometry.paddingTop - geometry.paddingBottom,
    ),
  ).toBeLessThanOrEqual(1);
  expect(geometry.nextGap).not.toBeNull();
  if (geometry.nextGap !== null) {
    expect(geometry.nextGap).toBeGreaterThanOrEqual(11);
    expect(geometry.nextGap).toBeLessThanOrEqual(13);
  }
}

/** Asserts the warning sits inside its row and inside the viewport width. */
export async function expectPermissionWarning(page: Page, copy: RegExp): Promise<void> {
  const { group, footer, warning } = notificationPermissionGroup(page);
  await expect(warning).toBeVisible();
  await expect(warning).toContainText(copy);
  await expect(footer).toHaveCount(1);
  await expect(footer.getByTestId("notification-permission-warning")).toHaveCount(1);
  const geometry = await warning.evaluate((element) => {
    const box = element.getBoundingClientRect();
    return { left: box.left, right: box.right, viewport: window.innerWidth };
  });
  expect(geometry.left).toBeGreaterThanOrEqual(0);
  expect(geometry.right).toBeLessThanOrEqual(geometry.viewport);
  expect((await requireBox(await group.boundingBox(), "permission row")).width).toBeLessThanOrEqual(
    geometry.viewport,
  );
}
