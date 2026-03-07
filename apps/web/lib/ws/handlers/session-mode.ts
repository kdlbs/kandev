import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { SessionModeChangedPayload } from "@/lib/types/backend";

export function registerSessionModeHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "session.mode_changed": (message) => {
      const payload = message.payload as SessionModeChangedPayload | undefined;
      if (!payload?.session_id) {
        return;
      }
      const sessionId = payload.session_id;
      // Empty current_mode_id means the agent exited its special mode
      const modeId = payload.current_mode_id || "";
      const availableModes = payload.available_modes?.map((m) => ({
        id: m.id,
        name: m.name,
        description: m.description,
      }));

      if (modeId) {
        store.getState().setSessionMode(sessionId, modeId, availableModes);
      } else {
        // Keep availableModes so the UI still knows what modes exist
        store.getState().setSessionMode(sessionId, "", availableModes);
      }
    },
  };
}
