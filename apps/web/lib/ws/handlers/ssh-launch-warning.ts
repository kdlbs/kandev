import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { LaunchWarningPayload } from "@/lib/types/backend";

export function registerSSHLaunchWarningHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "session.launch.warning": (message) => {
      const payload = message.payload as LaunchWarningPayload;
      if (!payload.session_id) return;
      store.getState().setLaunchWarning(payload.session_id, {
        executorId: payload.executor_id,
        host: payload.host,
        state: payload.state,
        reason: payload.reason,
        lastSuccessAt: payload.last_success_at,
      });
    },
  };
}
