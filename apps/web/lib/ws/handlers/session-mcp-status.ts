import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { SessionMCPStatusPayload } from "@/lib/types/backend";
import type { WsHandlers } from "@/lib/ws/handlers/types";

export function registerSessionMCPStatusHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "session.mcp_status_updated": (message) => {
      const payload = message.payload as SessionMCPStatusPayload | undefined;
      if (!payload?.session_id || !payload.history?.current?.attachment_attempt_id) return;
      store.getState().setSessionMCPStatus(payload.session_id, payload.history);
    },
  };
}
