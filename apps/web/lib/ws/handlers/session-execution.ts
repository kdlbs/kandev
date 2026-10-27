import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "./types";
import { registerSandboxApprovalHandlers } from "./sandbox-approvals";

export function registerSessionExecutionHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    ...registerSandboxApprovalHandlers(store),
    "session.execution_changed": ({ payload }) => {
      if (!payload) return;
      const current = store.getState().taskSessions.items[payload.session_id];
      if (
        !current ||
        current.task_id !== payload.task_id ||
        current.queue_incarnation_id !== payload.queue_incarnation_id
      ) {
        return;
      }
      store.getState().upsertTaskSessionFromEvent(payload.task_id, {
        ...current,
        execution_snapshot: payload.execution_snapshot,
        execution_observation: payload.execution_observation,
      });
    },
  };
}
