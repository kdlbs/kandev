import type { StoreApi } from "zustand";
import { createDebugLogger } from "@/lib/debug/log";
import { isTerminalToolCallStatus } from "@/lib/utils/tool-call-status";
import { parseTurnTimestamp } from "@/lib/state/slices/session/turn-actions";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import { agentProfileId, sessionId, taskId } from "@/lib/types/http";
import { maybeEmitEmptyTurnNotice } from "@/lib/ws/handlers/empty-turn-notice";
import type { MessageUpdateScheduler } from "@/lib/ws/handlers/messages";

const debug = createDebugLogger("session:turns");

/** Marks in-flight tool-call messages complete/errored when a turn completes. */
function completePendingToolCalls(store: StoreApi<AppState>, sessionId: string): void {
  const messages = store.getState().messages.bySession[sessionId];
  if (!messages) return;

  for (const message of messages) {
    if (message.type === "permission_request") continue;
    const metadata = message.metadata as Record<string, unknown> | undefined;
    if (metadata?.tool_call_id && !isTerminalToolCallStatus(String(metadata.status ?? ""))) {
      store.getState().updateMessage({
        ...message,
        metadata: { ...metadata, status: "complete" },
      });
    }
  }
}

/** Registers the session.turn.* WebSocket handlers (started/completed) against the store. */
export function registerTurnsHandlers(
  store: StoreApi<AppState>,
  messageScheduler?: Pick<MessageUpdateScheduler, "flush">,
): WsHandlers {
  return {
    "session.turn.started": (message) => {
      const payload = message.payload;
      if (!payload.session_id) {
        return;
      }
      debug("turn.started", {
        sessionId: payload.session_id,
        task_id: payload.task_id ?? "-",
        turnId: payload.id,
      });
      store.getState().addTurn({
        id: payload.id,
        session_id: sessionId(payload.session_id),
        task_id: taskId(payload.task_id),
        started_at: payload.started_at,
        completed_at: payload.completed_at,
        execution_profile_id: payload.execution_profile_id
          ? agentProfileId(payload.execution_profile_id)
          : undefined,
        route_generation: payload.route_generation,
        metadata: payload.metadata,
        created_at: payload.created_at,
        updated_at: payload.updated_at,
      });
      // Track this as the active turn for the session — unless the turn
      // STARTED at/before the session's settled boundary (source adoption,
      // settled-session clear). An authoritative boundary retired every turn
      // that predates it, even ones this client had not seen yet; a delayed
      // delivery of an old start must not resurrect the marker. A malformed
      // started_at is stale data and is not marked either.
      const boundary = parseTurnTimestamp(
        store.getState().turns.settledBoundaryBySession[payload.session_id],
      );
      const started = parseTurnTimestamp(payload.started_at);
      if (started !== null && (boundary === null || started > boundary)) {
        store.getState().setActiveTurn(payload.session_id, payload.id);
      }
    },
    "session.turn.completed": (message) => {
      const payload = message.payload;
      if (!payload.session_id || !payload.id) {
        return;
      }
      const wasCompleted = store
        .getState()
        .turns.bySession[
          payload.session_id
        ]?.some((turn) => turn.id === payload.id && Boolean(turn.completed_at));
      messageScheduler?.flush();
      debug("turn.completed", {
        sessionId: payload.session_id,
        task_id: payload.task_id ?? "-",
        turnId: payload.id,
        completedAt: payload.completed_at ?? "-",
      });
      store.getState().addTurn({
        id: payload.id,
        session_id: sessionId(payload.session_id),
        task_id: taskId(payload.task_id),
        started_at: payload.started_at,
        completed_at: payload.completed_at || new Date().toISOString(),
        execution_profile_id: payload.execution_profile_id
          ? agentProfileId(payload.execution_profile_id)
          : undefined,
        route_generation: payload.route_generation,
        metadata: payload.metadata,
        created_at: payload.created_at,
        updated_at: payload.updated_at,
      });
      store
        .getState()
        .completeTurn(
          payload.session_id,
          payload.id,
          payload.completed_at || new Date().toISOString(),
          payload.metadata,
          payload.updated_at,
        );
      const quickChat = store.getState().quickChat;
      const quickChatSession = quickChat.sessions.find(
        (session) => session.sessionId === payload.session_id,
      );
      if (quickChatSession && !quickChat.isOpen && !wasCompleted) {
        // Share the settled ledger with the state_changed path so a replay of
        // the same completion (reconnect, delayed event) cannot re-mark after
        // the user already saw the dot. completed_at is the stable identity of
        // this completion; a missing value falls through to the mark.
        if (
          !payload.completed_at ||
          store.getState().recordQuickChatSettled(payload.session_id, payload.completed_at)
        ) {
          store
            .getState()
            .markQuickChatUnseenIdle(payload.session_id, quickChatSession.workspaceId);
        }
      }
      // Surface a notice when the turn finished with no agent output.
      maybeEmitEmptyTurnNotice(store, payload);

      // Safety net: mark any tool calls still in a non-terminal state as "complete".
      // This handles edge cases where tool_update events were dropped or not processed.
      // Permission_request messages also carry `tool_call_id` in metadata, but their
      // `status` represents the user's approve/reject decision — not the tool call
      // state — so they must be excluded from the sweep. Forcing them to "complete"
      // wipes "approved"/"rejected" and re-shows the prompt buttons in the UI.
      completePendingToolCalls(store, payload.session_id);
    },
  };
}
