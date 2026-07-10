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
      const existingACP = readExistingACP(existing.metadata?.acp);
      if (isStaleSessionInfoUpdate(payload.session_updated_at, existingACP.updated_at)) return;
      store.getState().setTaskSession({
        ...existing,
        metadata: {
          ...(existing.metadata ?? {}),
          acp: {
            session_id: payload.acp_session_id || existingACP.session_id,
            title: payload.session_title || existingACP.title,
            updated_at: payload.session_updated_at || existingACP.updated_at,
            meta: payload.session_meta ?? existingACP.meta,
          },
        },
      });
    },
  };
}

function isStaleSessionInfoUpdate(
  incomingUpdatedAt: string | undefined,
  existingUpdatedAt: string,
) {
  if (!incomingUpdatedAt || !existingUpdatedAt) return false;
  const incoming = Date.parse(incomingUpdatedAt);
  const existing = Date.parse(existingUpdatedAt);
  if (Number.isNaN(incoming) || Number.isNaN(existing)) return false;
  return incoming < existing;
}

function readExistingACP(value: unknown) {
  if (!value || typeof value !== "object") {
    return { session_id: "", title: "", updated_at: "", meta: {} };
  }
  const record = value as Record<string, unknown>;
  return {
    session_id: typeof record.session_id === "string" ? record.session_id : "",
    title: typeof record.title === "string" ? record.title : "",
    updated_at: typeof record.updated_at === "string" ? record.updated_at : "",
    meta:
      record.meta && typeof record.meta === "object"
        ? (record.meta as Record<string, unknown>)
        : {},
  };
}
