import type { StoreApi } from "zustand";
import {
  NOTIFICATION_EVENT_OFFICE_INBOX_ITEM,
  NOTIFICATION_EVENT_SESSION_CLARIFICATION_REQUESTED,
  NOTIFICATION_EVENT_SESSION_TURN_FINISHED,
  NOTIFICATION_EVENT_SYSTEM_UPDATE_AVAILABLE,
} from "@/lib/notifications/events";
import { playWaitingForInputSound } from "@/lib/notifications/sound";
import { nativeNotifications } from "@/lib/desktop/native-notification-client";
import type { AppState } from "@/lib/state/store";
import type {
  OfficeInboxItemNotificationPayload,
  TaskSessionNotificationPayload,
  UpdateAvailablePayload,
} from "@/lib/types/backend";
import type { WsHandlers } from "@/lib/ws/handlers/types";

/** Check whether the notification should be suppressed. */
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

function registerNotificationHandler(
  store: StoreApi<AppState>,
  eventType: string,
  fallback: Pick<NotificationPayload, "title" | "body">,
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
      title: message.payload.title || fallback.title,
      body: message.payload.body || fallback.body,
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
        title: "Agent turn finished",
        body: "The agent finished a turn.",
      },
    ),
    [NOTIFICATION_EVENT_SESSION_CLARIFICATION_REQUESTED]: registerNotificationHandler(
      store,
      NOTIFICATION_EVENT_SESSION_CLARIFICATION_REQUESTED,
      {
        title: "Agent needs your answer",
        body: "The agent asked a question.",
      },
    ),
    [NOTIFICATION_EVENT_OFFICE_INBOX_ITEM]: registerNotificationHandler(
      store,
      NOTIFICATION_EVENT_OFFICE_INBOX_ITEM,
      {
        title: "New inbox item",
        body: "A new item needs your attention.",
      },
    ),
    [NOTIFICATION_EVENT_SYSTEM_UPDATE_AVAILABLE]: (message: { payload: UpdateAvailablePayload }) =>
      store.getState().setUpdateAvailableNotification(message.payload),
  };
}
