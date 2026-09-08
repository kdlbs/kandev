import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";

export function registerSessionHostnamesHandlers(store: StoreApi<AppState>): WsHandlers {
  const epoch = store.getState().sessionHostnamesEpoch;
  return {
    "auth.session.hostname.resolved": (message) => {
      if (store.getState().sessionHostnamesEpoch !== epoch) return;
      store
        .getState()
        .setSessionHostname(
          message.payload.ip,
          message.payload.hostname,
          message.payload.resolved_at,
        );
    },
  };
}
