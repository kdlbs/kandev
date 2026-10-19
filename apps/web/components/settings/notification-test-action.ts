import { nativeNotifications } from "@/lib/desktop/native-notification-client";
import { t } from "@/lib/i18n";
import { generateUUID } from "@/lib/uuid";
import type { PermissionRefresh } from "./notifications-settings-actions";

async function refreshPermission(refresh: PermissionRefresh, error?: unknown) {
  try {
    await refresh(error);
  } catch {
    // Permission feedback must not escape a user gesture handler.
  }
}

function reportDisplayFailure(error: unknown) {
  // i18n-exempt: Internal console diagnostic, not rendered notification copy.
  console.error("[NotificationsSettings] Test notification failed", error);
}

async function sendNativeTest(refresh: PermissionRefresh) {
  let permission;
  try {
    permission = await nativeNotifications.permission.get();
    if (permission === "prompt" || permission === "prompt-with-rationale") {
      permission = await nativeNotifications.permission.request();
      await refreshPermission(refresh);
    }
  } catch (error) {
    await refreshPermission(refresh, error);
    return;
  }
  if (permission !== "granted") {
    await refreshPermission(refresh);
    return;
  }
  try {
    const result = await nativeNotifications.show({
      // i18n-exempt: Private native diagnostic event identity, not user-facing copy.
      eventId: `desktop.test_notification:${generateUUID()}`,
      title: t("settings:testNotificationTitle"),
      body: t("settings:testNotificationBody"),
    });
    if (result === "permission-denied") {
      await refreshPermission(refresh);
    } else if (result === "display-failed") {
      reportDisplayFailure(result);
      await refreshPermission(refresh);
    }
  } catch (error) {
    reportDisplayFailure(error);
    await refreshPermission(refresh);
  }
}

async function sendBrowserTest(refresh: PermissionRefresh) {
  if (typeof Notification === "undefined") return;
  let permission = Notification.permission;
  if (permission === "default") {
    try {
      permission = await Notification.requestPermission();
      await refreshPermission(refresh);
    } catch (error) {
      await refreshPermission(refresh, error);
      return;
    }
  }
  if (permission !== "granted") return;
  try {
    new Notification(t("settings:testNotificationTitle"), {
      body: t("settings:testNotificationBody"),
    });
  } catch (error) {
    reportDisplayFailure(error);
    await refreshPermission(refresh);
  }
}

export async function sendTestNotification(refresh: PermissionRefresh): Promise<void> {
  if (nativeNotifications.isAvailable()) {
    await sendNativeTest(refresh);
    return;
  }
  await sendBrowserTest(refresh);
}
