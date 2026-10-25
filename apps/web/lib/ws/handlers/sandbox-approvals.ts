import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "./types";

const stores = new WeakMap<StoreApi<AppState>, Map<string, Set<() => void>>>();

export function subscribeSandboxApprovalChanges(
  store: StoreApi<AppState>,
  sessionId: string,
  listener: () => void,
): () => void {
  let sessions = stores.get(store);
  if (!sessions) {
    sessions = new Map();
    stores.set(store, sessions);
  }
  let listeners = sessions.get(sessionId);
  if (!listeners) {
    listeners = new Set();
    sessions.set(sessionId, listeners);
  }
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
    if (listeners.size === 0) sessions.delete(sessionId);
    if (sessions.size === 0) stores.delete(store);
  };
}

export function registerSandboxApprovalHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "session.sandbox_approvals_changed": ({ payload }) => {
      if (!payload) return;
      const current = store.getState().taskSessions.items[payload.session_id];
      if (
        !current ||
        current.task_id !== payload.task_id ||
        current.queue_incarnation_id !== payload.queue_incarnation_id
      )
        return;
      for (const listener of stores.get(store)?.get(payload.session_id) ?? []) listener();
    },
  };
}
