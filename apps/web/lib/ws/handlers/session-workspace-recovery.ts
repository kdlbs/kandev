import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { SessionWorkspaceRecoveryChangedPayload } from "@/lib/types/session-events";
import type { WsHandlers } from "@/lib/ws/handlers/types";

function targetSessionIds(payload: SessionWorkspaceRecoveryChangedPayload): string[] {
  if (payload.session_ids?.length) return payload.session_ids;
  if (payload.session_id) return [payload.session_id];
  return [];
}

export function registerSessionWorkspaceRecoveryHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "session.workspace_recovery.changed": (message) => {
      const payload = message.payload as SessionWorkspaceRecoveryChangedPayload | undefined;
      if (!payload?.environment_id || !payload.workspace_recovery) return;
      store
        .getState()
        .setWorkspaceRecoveryProjection(targetSessionIds(payload), payload.workspace_recovery);
    },
  };
}
