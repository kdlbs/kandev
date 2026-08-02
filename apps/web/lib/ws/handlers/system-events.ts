import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import { handleBrowserLogCapture } from "@/lib/logger/capture";

export function registerSystemEventsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "system.job.update": (message) => {
      // The WS payload is the full SystemJob row published by the backend
      // jobs tracker (see internal/system/jobs). Upsert by id so the
      // jobs map mirrors the latest queued/running/succeeded/failed state.
      store.getState().upsertSystemJob(message.payload);
    },
    "system.metrics.updated": (message) => {
      store.getState().setSystemMetricsSnapshot(message.payload);
    },
    "system.logs.capture_requested": (message) => {
      const auth = store.getState().auth;
      const identityScope = auth.authenticated
        ? (auth.user?.id ?? (auth.mode === "disabled" ? "default-user" : null))
        : null;
      void handleBrowserLogCapture(message.payload, identityScope).catch(() => {
        // Capture errors must not recurse through the intercepted console.
      });
    },
  };
}
