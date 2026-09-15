import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { SessionPendingActionChangedPayload } from "@/lib/types/backend";
import type { WsHandlers } from "@/lib/ws/handlers/types";

export function registerSessionPendingActionHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "session.pending_action_changed": (message) => {
      const payload = message.payload as SessionPendingActionChangedPayload | undefined;
      if (!payload?.workspace_id || !payload.task_id || !payload.session_id) return;
      store
        .getState()
        .setTaskSessionPendingAction(
          payload.session_id,
          payload.pending_action,
          payload.pending_action_revision,
          payload.task_id,
        );
    },
  };
}
