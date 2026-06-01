import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { SessionModelsPayload } from "@/lib/types/backend";

/**
 * Session models live in the TanStack Query cache now
 * (qk.session.models(sessionId), written by bridge/session-runtime.ts). This
 * Zustand handler is retained only for its client-state side effect: clearing a
 * stale `activeModel` (the user's locally-selected model) when the agent's ACP
 * model list arrives and no longer contains the selected ID.
 */
export function registerSessionModelsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "session.models_updated": (message) => {
      const payload = message.payload as SessionModelsPayload | undefined;
      if (!payload?.session_id) {
        return;
      }
      const acpModels = payload.models ?? [];

      // Clear stale activeModel if it uses a profile ID that doesn't exist in ACP models.
      // This happens when a user selected a static model before ACP models arrived.
      if (acpModels.length > 0) {
        const state = store.getState();
        const currentActive = state.activeModel.bySessionId[payload.session_id];
        if (currentActive && !acpModels.some((m) => m.model_id === currentActive)) {
          state.setActiveModel(payload.session_id, "");
        }
      }
    },
  };
}
