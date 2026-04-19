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
      });
      if (typeof Notification === "undefined") return;
      if (Notification.permission !== "granted") return;

      // Only notify after the agent has completed at least one turn.
      // During initial task creation the session reaches WAITING_FOR_INPUT
      // before the agent starts — that's not a genuine "waiting for input".
      if (sessionId) {
        const turns = state.turns.bySession[sessionId];
        if (!turns || turns.length === 0) {
          console.log("[notification] suppressed — session has no completed turns yet");
          return;
        }
      }

      // Suppress when user is already viewing this task
      if (document.visibilityState === "visible") {
        if (taskId && state.tasks.activeTaskId === taskId) {
          console.log("[notification] suppressed — user is viewing this task");
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
