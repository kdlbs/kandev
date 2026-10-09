import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";

export function registerPreviewFeedbackHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "task.preview_feedback.changed": (message) => {
      store.getState().setTaskPreviewFeedback(message.payload.task_id, message.payload);
    },
  };
}
