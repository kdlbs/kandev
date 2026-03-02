import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";

export function registerSessionModeHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "session.mode_changed": (message) => {
      const payload = message.payload;
      if (!payload?.session_id) {
        return;
      }
      const sessionId = payload.session_id as string;
      // Empty current_mode_id means the agent exited its special mode
      const modeId = (payload.current_mode_id as string) || "";

      if (modeId) {
        store.getState().setSessionMode(sessionId, modeId);
      } else {
        store.getState().clearSessionMode(sessionId);
      }
    },
  };
}
