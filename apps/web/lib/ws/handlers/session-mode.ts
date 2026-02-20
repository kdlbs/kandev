import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";

export function registerSessionModeHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "session.mode_changed": (message) => {
      const payload = message.payload;
      if (!payload?.session_id || !payload?.current_mode_id) {
        return;
      }
      const sessionId = payload.session_id as string;
      const modeId = payload.current_mode_id as string;

      store.getState().setSessionMode(sessionId, modeId);
    },
  };
}
