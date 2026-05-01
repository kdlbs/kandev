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

      // Safety net: mark any tool calls still in a non-terminal state as "complete".
      // This handles edge cases where tool_update events were dropped or not processed.
      // Permission_request messages also carry `tool_call_id` in metadata, but their
      // `status` represents the user's approve/reject decision — not the tool call
      // state — so they must be excluded from the sweep. Forcing them to "complete"
      // wipes "approved"/"rejected" and re-shows the prompt buttons in the UI.
      const messages = store.getState().messages.bySession[payload.session_id];
      if (messages) {
        for (const msg of messages) {
          if (msg.type === "permission_request") continue;
          const meta = msg.metadata as Record<string, unknown> | undefined;
          if (meta?.tool_call_id && meta?.status !== "complete" && meta?.status !== "error") {
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
