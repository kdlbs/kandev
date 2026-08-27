import type { StoreApi } from "zustand";
import type { TaskSessionState } from "@/lib/types/http";
import type { AppState } from "@/lib/state/store";

const ACTIVE_STATES: ReadonlySet<TaskSessionState> = new Set(["STARTING", "RUNNING"]);
const SETTLED_STATES: ReadonlySet<TaskSessionState> = new Set([
  "IDLE",
  "WAITING_FOR_INPUT",
  "COMPLETED",
  "FAILED",
  "CANCELLED",
]);

/**
 * How long a `session.state_changed` settle may lag the `session.turn.completed`
 * for the same logical completion before it counts as new activity. The backend
 * emits the turn event first and the session-row update a few milliseconds
 * later, so the state_changed that follows a turn completion is its echo, not a
 * second completion — marking it again would re-arm a dot the user already
 * viewed. Genuine later completions land seconds or minutes afterwards and are
 * unaffected.
 */
const TURN_STATE_ECHO_GRACE_MS = 2000;

/** Whether the settle is the state_changed echo of the completion just recorded. */
function isTurnCompletionEcho(lastSettledAt: string | undefined, updatedAt: string): boolean {
  if (!lastSettledAt) return false;
  const settleTime = Date.parse(updatedAt);
  const lastTime = Date.parse(lastSettledAt);
  if (!Number.isFinite(settleTime) || !Number.isFinite(lastTime)) return false;
  const delta = settleTime - lastTime;
  return delta >= 0 && delta <= TURN_STATE_ECHO_GRACE_MS;
}

export function maybeMarkQuickChatUnseenIdle(
  store: StoreApi<AppState>,
  sessionId: string,
  transition: {
    previousState: TaskSessionState | undefined;
    fallbackPreviousState: TaskSessionState | undefined;
    newState: TaskSessionState | undefined;
    updatedAt: string | undefined;
  },
): void {
  const { previousState, fallbackPreviousState, newState, updatedAt } = transition;
  const priorState = previousState ?? fallbackPreviousState;
  if (
    !priorState ||
    !newState ||
    !ACTIVE_STATES.has(priorState) ||
    !SETTLED_STATES.has(newState) ||
    !updatedAt
  )
    return;
  const quickChat = store.getState().quickChat;
  // The settle ledger is consulted before recording so a state_changed that
  // echoes a turn completion (recorded moments earlier with a slightly older
  // timestamp) is recognized as the same logical completion instead of new.
  if (isTurnCompletionEcho(quickChat.lastSettledAtBySession?.[sessionId], updatedAt)) return;
  if (!store.getState().recordQuickChatSettled(sessionId, updatedAt)) return;
  const session = quickChat.sessions.find((item) => item.sessionId === sessionId);
  if (session && !quickChat.isOpen)
    store.getState().markQuickChatUnseenIdle(sessionId, session.workspaceId);
}
