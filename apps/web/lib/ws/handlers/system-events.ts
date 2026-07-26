import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";

export function registerSystemEventsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "system.error": () => {
      // TODO: surface as toast/notification once UI is ready.
    },
    "system.job.update": (message) => {
      // The WS payload is the full SystemJob row published by the backend
      // jobs tracker (see internal/system/jobs). Upsert by id so the
      // jobs map mirrors the latest queued/running/succeeded/failed state.
      store.getState().upsertSystemJob(message.payload);
    },
    "system.metrics.updated": (message) => {
      store.getState().setSystemMetricsSnapshot(message.payload);
    },
  };
}
