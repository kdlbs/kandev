import {
  readMcpAttachmentHistory,
  shouldReplaceMcpAttachmentHistory,
} from "@/lib/state/slices/session-runtime/mcp-attachment-reconciliation";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { SessionMCPStatusPayload } from "@/lib/types/backend";
import type { WsHandlers } from "@/lib/ws/handlers/types";

export function registerSessionMCPStatusHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "session.mcp_status_updated": (message) => {
      const payload = message.payload as SessionMCPStatusPayload | undefined;
      if (!payload?.session_id || !payload.history?.current?.attachment_attempt_id) return;
      const history = readMcpAttachmentHistory(payload.history);
      if (!history) return;
      if (history.current.session_id && history.current.session_id !== payload.session_id) return;
      if (history.current.task_id && history.current.task_id !== payload.task_id) return;
      const state = store.getState();
      if (
        !shouldReplaceMcpAttachmentHistory(
          state.sessionMcpStatus?.bySessionId[payload.session_id],
          history,
        )
      )
        return;
      state.setSessionMCPStatus(payload.session_id, history);
    },
  };
}
