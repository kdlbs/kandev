import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";

export function registerTaskPlanCommentsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "task.plan.comments.changed": (message) => {
      store.getState().setTaskPlanComments(message.payload.task_id, message.payload);
    },
  };
}
