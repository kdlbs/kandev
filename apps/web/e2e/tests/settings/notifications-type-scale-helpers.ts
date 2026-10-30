import { expect } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";

const TURN_FINISHED = "session.turn_finished";
const CLARIFICATION_REQUESTED = "session.clarification_requested";

/** Two providers so the desktop events table renders more than one column. */
export const PROVIDER_NAMES = ["Desktop", "Team channel"];

/** Permission values the settings page can read from `Notification.permission`. */
export type StubbedNotificationPermission = "default" | "denied" | "granted";

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
    warning: group.getByTestId("notification-permission-warning"),
    card: group.locator('[data-settings-group-card="true"]'),
    content: group.locator('[data-settings-group-content="true"]'),
    refresh: group.getByRole("button", { name: "Refresh notification permission" }),
  };
}

/** Asserts the undecided/granted composition: no card, hidden body, normal gap. */
export async function expectNormalPermissionLayout(page: Page): Promise<void> {
  const { group, warning, card, content } = notificationPermissionGroup(page);
  await expect(group).toBeVisible();
  await expect(warning).toHaveCount(0);
  await expect(card).toHaveCount(0);
  // `display: none` is stronger than Playwright's zero-box "hidden" heuristic:
  // the empty body element must not participate in layout at all.
  await expect(content).toHaveCSS("display", "none");
  const padding = await content.evaluate((element) => {
    const style = getComputedStyle(element);
    return [style.paddingTop, style.paddingBottom];
  });
  expect(padding).toEqual(["0px", "0px"]);
  const gap = await group.evaluate((element) => {
    const header = element.firstElementChild;
    const next = element.nextElementSibling;
    if (!header || !next) return null;
    return next.getBoundingClientRect().top - header.getBoundingClientRect().bottom;
  });
  expect(gap).not.toBeNull();
  if (gap !== null) {
    expect(gap).toBeGreaterThanOrEqual(23);
    expect(gap).toBeLessThanOrEqual(25);
  }
}

/** Asserts the warning sits inside its card and inside the viewport width. */
export async function expectPermissionWarning(page: Page, copy: RegExp): Promise<void> {
  const { warning, card } = notificationPermissionGroup(page);
  await expect(warning).toBeVisible();
  await expect(warning).toContainText(copy);
  await expect(card).toHaveCount(1);
  await expect(card.getByTestId("notification-permission-warning")).toHaveCount(1);
  const geometry = await warning.evaluate((element) => {
    const box = element.getBoundingClientRect();
    return { left: box.left, right: box.right, viewport: window.innerWidth };
  });
  expect(geometry.left).toBeGreaterThanOrEqual(0);
  expect(geometry.right).toBeLessThanOrEqual(geometry.viewport);
}
