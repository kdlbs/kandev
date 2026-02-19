import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { MessageType } from "@/lib/types/http";

export function registerMessagesHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "session.message.added": (message) => {
      const payload = message.payload;
      if (!payload.session_id) {
        return;
      }
      store.getState().addMessage({
        id: payload.message_id,
        session_id: payload.session_id,
        task_id: payload.task_id,
        turn_id: payload.turn_id,
        author_type: payload.author_type,
        author_id: payload.author_id,
        content: payload.content,
        type: (payload.type as MessageType) || "message",
        metadata: payload.metadata,
        requests_input: payload.requests_input,
        created_at: payload.created_at,
      });
    },
    "session.message.updated": (message) => {
      const payload = message.payload;
      if (!payload.session_id) {
        return;
      }
      store.getState().updateMessage({
        id: payload.message_id,
        session_id: payload.session_id,
        task_id: payload.task_id,
        turn_id: payload.turn_id,
        author_type: payload.author_type,
        author_id: payload.author_id,
        content: payload.content,
        type: (payload.type as MessageType) || "message",
        metadata: payload.metadata,
        requests_input: payload.requests_input,
        created_at: payload.created_at,
      });
    },
  };
}
