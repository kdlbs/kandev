import type { StoreApi } from "zustand";
import {
  NOTIFICATION_EVENT_OFFICE_INBOX_ITEM,
  NOTIFICATION_EVENT_SESSION_CLARIFICATION_REQUESTED,
  NOTIFICATION_EVENT_SESSION_TURN_FINISHED,
  NOTIFICATION_EVENT_SYSTEM_UPDATE_AVAILABLE,
} from "@/lib/notifications/events";
import { playWaitingForInputSound } from "@/lib/notifications/sound";
// Module-level `t`, resolved at call time: these handlers are plain callbacks
// invoked by the WS client, not components, so there is no hook to bind.
import { t } from "@/lib/i18n";
import { nativeNotifications } from "@/lib/desktop/native-notification-client";
import type { AppState } from "@/lib/state/store";
import type {
  OfficeInboxItemNotificationPayload,
  TaskSessionNotificationPayload,
  UpdateAvailablePayload,
} from "@/lib/types/backend";
import type { WsHandlers } from "@/lib/ws/handlers/types";

/** Check whether the notification should be suppressed. */
// i18n-exempt: the returned reason is used only for truthiness at the call
// site; it is never rendered or logged.
function shouldSuppressNotification(state: AppState, taskId: string | undefined): string | null {
  // Suppress when user is actively viewing this task.
  if (document.visibilityState === "visible" && taskId && state.tasks.activeTaskId === taskId) {
    return "user is viewing this task";
  }
  return null;
}

/** Show the desktop notification when the browser permission allows it. */
type NotificationPayload = TaskSessionNotificationPayload | OfficeInboxItemNotificationPayload;

function isSemanticSessionNotification(eventType: string): boolean {
  return (
    eventType === NOTIFICATION_EVENT_SESSION_TURN_FINISHED ||
    eventType === NOTIFICATION_EVENT_SESSION_CLARIFICATION_REQUESTED
  );
}

function showDesktopNotification(payload: NotificationPayload): void {
  if (typeof Notification === "undefined" || Notification.permission !== "granted") return;
  new Notification(payload.title, { body: payload.body });
}

/**
 * Catalog keys for the copy shown when the backend sends an empty title/body.
 * Keys rather than resolved strings: the handlers are registered once when the
 * WS client starts, so resolving here would pin the fallback to whichever locale
 * was active at registration. `t` runs inside the handler, per notification.
 */
type NotificationFallbackKeys = { titleKey: string; bodyKey: string };

function registerNotificationHandler(
  store: StoreApi<AppState>,
  eventType: string,
  fallback: NotificationFallbackKeys,
) {
  return (message: { id?: string; payload: NotificationPayload }) => {
    const sessionId = message.payload?.session_id;
    const taskId = message.payload?.task_id;
    const state = store.getState();

    const reason = shouldSuppressNotification(state, taskId);
    if (reason) return;

    playWaitingForInputSound();
    const payload = {
      ...message.payload,
      title: message.payload.title || t(fallback.titleKey),
      body: message.payload.body || t(fallback.bodyKey),
    };
    const occurrenceID =
      message.id ??
      ("occurrence_id" in message.payload ? message.payload.occurrence_id : undefined);
    const nativeEventID = isSemanticSessionNotification(eventType)
      ? occurrenceID
      : (message.id ?? `${taskId}:${sessionId ?? "unknown"}`);
    if (nativeNotifications.isAvailable() && taskId && nativeEventID) {
      void nativeNotifications
        .show({
          eventId: `${eventType}:${nativeEventID}`,
          title: payload.title,
          body: payload.body,
          taskId,
          sessionId,
        })
        .catch(() => undefined);
    } else {
      showDesktopNotification(payload);
    }
  };
}

export function registerNotificationsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    [NOTIFICATION_EVENT_SESSION_TURN_FINISHED]: registerNotificationHandler(
      store,
      NOTIFICATION_EVENT_SESSION_TURN_FINISHED,
      {
        titleKey: "common:notificationTurnFinishedTitle",
        bodyKey: "common:notificationTurnFinishedBody",
      },
    ),
    [NOTIFICATION_EVENT_SESSION_CLARIFICATION_REQUESTED]: registerNotificationHandler(
      store,
      NOTIFICATION_EVENT_SESSION_CLARIFICATION_REQUESTED,
      {
        titleKey: "common:notificationClarificationTitle",
        bodyKey: "common:notificationClarificationBody",
      },
    ),
    [NOTIFICATION_EVENT_OFFICE_INBOX_ITEM]: registerNotificationHandler(
      store,
      NOTIFICATION_EVENT_OFFICE_INBOX_ITEM,
      {
        titleKey: "common:notificationInboxItemTitle",
        bodyKey: "common:notificationInboxItemBody",
      },
    ),
    [NOTIFICATION_EVENT_SYSTEM_UPDATE_AVAILABLE]: (message: { payload: UpdateAvailablePayload }) =>
      store.getState().setUpdateAvailableNotification(message.payload),
  };
}
