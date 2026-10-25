import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";

export function registerTurnChangesHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "session.turn.changes.updated": (message) => {
      const { session_id: sessionId, change_set: summary } = message.payload;
      if (!sessionId || !summary?.id) return;
      store.getState().mergeTurnChangeSummary(sessionId, summary);
    },
  };
}
