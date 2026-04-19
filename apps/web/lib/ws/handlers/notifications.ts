import type { StoreApi } from "zustand";
import { NOTIFICATION_EVENT_TASK_SESSION_WAITING_FOR_INPUT } from "@/lib/notifications/events";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";

export function registerNotificationsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    [NOTIFICATION_EVENT_TASK_SESSION_WAITING_FOR_INPUT]: (message) => {
      const sessionId = message.payload?.session_id as string | undefined;
      const taskId = message.payload?.task_id as string | undefined;
      const state = store.getState();
      console.log("[notification] session.waiting_for_input received", {
        taskId,
        sessionId,
        activeTaskId: state.tasks.activeTaskId,
        activeSessionId: state.tasks.activeSessionId,
        visibility: document.visibilityState,
        notificationPermission: typeof Notification !== "undefined" ? Notification.permission : "N/A",
        sessionState: sessionId ? state.taskSessions.items[sessionId]?.state : "unknown",
      });
      if (typeof Notification === "undefined") {
        return;
      }
      if (Notification.permission !== "granted") {
        return;
      }
      // Suppress notification when user is actively viewing the task
      if (document.visibilityState === "visible") {
        if (taskId && state.tasks.activeTaskId === taskId) {
          console.log("[notification] suppressed — user is viewing this task");
          return;
        }
        if (sessionId && state.tasks.activeSessionId === sessionId) {
          console.log("[notification] suppressed — session is active");
          return;
        }
      }
      const title = message.payload.title || "Task needs your input";
      const body = message.payload.body || "An agent is waiting for your input.";
      console.log("[notification] firing browser notification:", title);
      new Notification(title, { body });
    },
  };
}
