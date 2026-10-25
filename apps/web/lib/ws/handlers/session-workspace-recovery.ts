import type { StoreApi } from "zustand";
import { fetchTaskSession } from "@/lib/api/domains/session-api";
import { captureTaskSessionHydrationEpoch } from "@/lib/state/slices/session/hydration-epochs";
import type { AppState } from "@/lib/state/store";
import type { SessionWorkspaceRecoveryChangedPayload } from "@/lib/types/session-events";
import type { WsHandlers } from "@/lib/ws/handlers/types";

function targetSessionIds(payload: SessionWorkspaceRecoveryChangedPayload): string[] {
  if (payload.session_ids?.length) return payload.session_ids;
  if (payload.session_id) return [payload.session_id];
  return [];
}

function refreshSessionInventory(
  store: StoreApi<AppState>,
  sessionId: string,
  payload: SessionWorkspaceRecoveryChangedPayload,
): void {
  const initialState = store.getState();
  const current = initialState.taskSessions.items[sessionId];
  const operation = payload.workspace_recovery;
  if (
    !current ||
    current.task_id !== payload.task_id ||
    current.task_environment_id !== payload.environment_id ||
    current.workspace_recovery?.operation_id !== operation?.operation_id ||
    current.workspace_recovery?.attempt_id !== operation?.attempt_id ||
    current.workspace_recovery.state === "running"
  ) {
    return;
  }
  const hydrationEpoch = captureTaskSessionHydrationEpoch(initialState, sessionId);

  void fetchTaskSession(sessionId)
    .then(({ session }) => {
      if (
        session.id !== sessionId ||
        session.task_id !== payload.task_id ||
        (session.task_environment_id ?? session.environment_id) !== payload.environment_id
      ) {
        return;
      }
      const latest = store.getState().taskSessions.items[sessionId];
      if (
        latest?.task_id !== payload.task_id ||
        latest.task_environment_id !== payload.environment_id ||
        latest.workspace_recovery?.operation_id !== operation?.operation_id ||
        latest.workspace_recovery?.attempt_id !== operation?.attempt_id ||
        latest.workspace_recovery.state === "running"
      ) {
        return;
      }
      store.getState().setTaskSession(session, hydrationEpoch);
    })
    .catch(() => {});
}

export function registerSessionWorkspaceRecoveryHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "session.workspace_recovery.changed": (message) => {
      const payload = message.payload as SessionWorkspaceRecoveryChangedPayload | undefined;
      if (!payload?.environment_id || !payload.workspace_recovery) return;
      const sessionIds = targetSessionIds(payload);
      const state = store.getState();
      state.setWorkspaceRecoveryProjection(sessionIds, payload.workspace_recovery);
      if (payload.workspace_recovery.state !== "running") {
        for (const sessionId of new Set(sessionIds)) {
          state.bumpWorkspaceFilesRefresh(sessionId);
          refreshSessionInventory(store, sessionId, payload);
        }
      }
    },
  };
}
