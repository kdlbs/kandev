import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { SessionInfoPayload } from "@/lib/types/backend";
import type { WsHandlers } from "@/lib/ws/handlers/types";

export function registerSessionInfoHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "session.info_updated": (message) => {
      const payload = message.payload as SessionInfoPayload | undefined;
      if (!payload?.session_id) return;
      const existing = store.getState().taskSessions.items[payload.session_id];
      if (!existing) return;
      store.getState().setTaskSession({
        ...existing,
        metadata: {
          ...(existing.metadata ?? {}),
          acp: {
            session_id: payload.acp_session_id ?? "",
            title: payload.session_title ?? "",
            updated_at: payload.session_updated_at ?? "",
            meta: payload.session_meta ?? {},
          },
        },
        updated_at: payload.timestamp ?? existing.updated_at,
      });
    },
  };
}
