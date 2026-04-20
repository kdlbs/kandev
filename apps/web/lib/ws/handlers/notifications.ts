import type { StoreApi } from "zustand";
import { NOTIFICATION_EVENT_TASK_SESSION_WAITING_FOR_INPUT } from "@/lib/notifications/events";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";

/** Check whether the notification should be suppressed. */
function shouldSuppressNotification(
  state: AppState,
  taskId: string | undefined,
  sessionId: string | undefined,
): string | null {
  // Suppress during initial preparation — session has no completed turns yet.
  if (sessionId) {
    const turns = state.turns.bySession[sessionId];
    if (!turns || turns.length === 0) return "session has no completed turns";
  }
  // Suppress when user is actively viewing this task.
  if (document.visibilityState === "visible" && taskId && state.tasks.activeTaskId === taskId) {
    return "user is viewing this task";
  }
  return null;
}

export function registerNotificationsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    [NOTIFICATION_EVENT_TASK_SESSION_WAITING_FOR_INPUT]: (message) => {
      if (typeof Notification === "undefined" || Notification.permission !== "granted") return;

      const sessionId = message.payload?.session_id as string | undefined;
      const taskId = message.payload?.task_id as string | undefined;
      const state = store.getState();

      const reason = shouldSuppressNotification(state, taskId, sessionId);
      if (reason) return;

      const title = message.payload.title || "Task needs your input";
      const body = message.payload.body || "An agent is waiting for your input.";
      new Notification(title, { body });
    },
  };
}
