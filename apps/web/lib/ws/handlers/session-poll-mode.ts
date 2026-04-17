import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { SessionPollMode } from "@/lib/state/slices/session-runtime/types";

const VALID_MODES = new Set<SessionPollMode>(["fast", "slow", "paused"]);

export function registerSessionPollModeHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "session.poll_mode_changed": (message) => {
      const { session_id, poll_mode } = message.payload;
      if (!session_id || !poll_mode) return;
      if (!VALID_MODES.has(poll_mode as SessionPollMode)) return;
      store.getState().setSessionPollMode(session_id, poll_mode as SessionPollMode);
    },
  };
}
