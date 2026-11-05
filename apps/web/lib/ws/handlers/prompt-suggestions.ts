import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { SessionPromptSuggestionPayload } from "@/lib/types/backend";
import type { WsHandlers } from "@/lib/ws/handlers/types";

/** Stores native next-prompt suggestions on the session, where hydration also restores them. */
export function registerPromptSuggestionHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "session.prompt_suggestion": (message) => {
      const payload = message.payload as SessionPromptSuggestionPayload | undefined;
      const text = payload?.text?.trim();
      if (!payload?.session_id || !payload.turn_id || !text) return;
      const existing = store.getState().taskSessions.items[payload.session_id];
      if (!existing) return;
      store.getState().setTaskSession({
        ...existing,
        metadata: {
          ...(existing.metadata ?? {}),
          prompt_suggestion: { turn_id: payload.turn_id, text },
        },
      });
    },
  };
}
