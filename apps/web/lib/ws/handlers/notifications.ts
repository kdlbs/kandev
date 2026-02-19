import type { StoreApi } from "zustand";
import { NOTIFICATION_EVENT_TASK_SESSION_WAITING_FOR_INPUT } from "@/lib/notifications/events";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";

export function registerNotificationsHandlers(_store: StoreApi<AppState>): WsHandlers {
  void _store;
  return {
    [NOTIFICATION_EVENT_TASK_SESSION_WAITING_FOR_INPUT]: (message) => {
      if (typeof Notification === "undefined") {
        return;
      }
      if (Notification.permission !== "granted") {
        return;
      }
      const title = message.payload.title || "Task needs your input";
      const body = message.payload.body || "An agent is waiting for your input.";
      new Notification(title, { body });
    },
  };
}
