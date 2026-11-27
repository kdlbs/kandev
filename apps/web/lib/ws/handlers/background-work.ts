import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { WorkloadRunObservation, WorkloadOutputChunk } from "@/lib/types/background-work";

export function registerBackgroundWorkHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "session.background_work.updated": (message) => {
      const payload = message.payload as WorkloadRunObservation | undefined;
      if (!payload?.session_id || !payload.work_id) return;
      store.getState().updateBackgroundWorkload(payload.session_id, payload);
    },
    "session.background_work.output": (message) => {
      const payload = message.payload as WorkloadOutputChunk | undefined;
      if (!payload?.session_id || !payload.work_id) return;
      store.getState().appendBackgroundWorkloadOutput(payload.session_id, payload);
    },
  };
}
