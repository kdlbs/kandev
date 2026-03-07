import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { SessionTodosPayload } from "@/lib/types/backend";
import type { TodoEntry } from "@/lib/state/slices/session-runtime/types";

export function registerSessionTodosHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "session.todos_updated": (message) => {
      const payload = message.payload as SessionTodosPayload | undefined;
      if (!payload?.session_id) {
        return;
      }
      store.getState().setSessionTodos(
        payload.session_id,
        (payload.entries ?? []).map(
          (e): TodoEntry => ({
            description: e.description,
            status: e.status as TodoEntry["status"],
            priority: e.priority,
          }),
        ),
      );
    },
  };
}
