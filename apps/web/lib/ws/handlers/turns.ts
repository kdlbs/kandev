import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";

export function registerTurnsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "session.turn.started": (message) => {
      const payload = message.payload;
      if (!payload.session_id) {
        return;
      }
      store.getState().addTurn({
        id: payload.id,
        session_id: payload.session_id,
        task_id: payload.task_id,
        started_at: payload.started_at,
        completed_at: payload.completed_at,
        metadata: payload.metadata,
        created_at: payload.created_at,
        updated_at: payload.updated_at,
      });
      // Track this as the active turn for the session
      store.getState().setActiveTurn(payload.session_id, payload.id);
    },
    "session.turn.completed": (message) => {
      const payload = message.payload;
      if (!payload.session_id || !payload.id) {
        return;
      }
      store
        .getState()
        .completeTurn(
          payload.session_id,
          payload.id,
          payload.completed_at || new Date().toISOString(),
        );
      // Clear the active turn when it completes
      store.getState().setActiveTurn(payload.session_id, null);

      // Safety net: mark any tool calls still in "running" state as "complete".
      // This handles edge cases where tool_update events were dropped or not processed.
      const messages = store.getState().messages.bySession[payload.session_id];
      if (messages) {
        for (const msg of messages) {
          const meta = msg.metadata as Record<string, unknown> | undefined;
          if (meta?.status === "running" && meta?.tool_call_id) {
            store.getState().updateMessage({
              ...msg,
              metadata: { ...meta, status: "complete" },
            });
          }
        }
      }
    },
  };
}
