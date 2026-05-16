import type { Page } from "@playwright/test";

/**
 * Captured payload of one `new Notification(title, opts)` call that the app
 * attempted while the page was alive.
 */
export type CapturedNotification = {
  title: string;
  body?: string;
};

/**
 * The e2e setupPage fixture overrides `window.Notification` with a stub that
 * never displays a real OS-level toast (those are noisy on developer
 * machines - Chromium pops the macOS notification center on every agent
 * "waiting for input" event during a run). The stub instead records each
 * attempted notification on `window.__kandevTestNotifications` so tests can
 * assert on what *would* have notified.
 *
 * Stub also reports `permission === "granted"` so the WS handler at
 * apps/web/lib/ws/handlers/notifications.ts (which early-returns when
 * permission is not granted) still runs its logic end to end.
 */
declare global {
  interface Window {
    __kandevTestNotifications?: CapturedNotification[];
  }
}

export async function getCapturedNotifications(page: Page): Promise<CapturedNotification[]> {
  return page.evaluate(() => window.__kandevTestNotifications ?? []);
}

export async function clearCapturedNotifications(page: Page): Promise<void> {
  await page.evaluate(() => {
    if (window.__kandevTestNotifications) window.__kandevTestNotifications.length = 0;
  });
}
