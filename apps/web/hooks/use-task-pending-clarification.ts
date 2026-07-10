import { useAppStore } from "@/components/state-provider";
import { hasPendingClarificationForSession } from "@/lib/utils/pending-clarification";

type TaskPendingSnapshot = {
  primarySessionId?: string | null;
  primarySessionState?: string | null;
  primarySessionPendingAction?: "clarification" | "permission" | null;
};

/**
 * Whether the task's primary session is blocked on a pending clarification.
 * Message-derived state wins once the session's messages are loaded; until
 * then (fresh page load, task never opened) fall back to the
 * `primary_session_pending_action` snapshot the backend denormalizes onto the
 * task, gated on the session still being WAITING_FOR_INPUT.
 */
export function useTaskPendingClarification(task: TaskPendingSnapshot): boolean {
  const { primarySessionId, primarySessionState, primarySessionPendingAction } = task;
  return useAppStore((state) => {
    if (primarySessionId && state.messages.bySession[primarySessionId] !== undefined) {
      return hasPendingClarificationForSession(state.messages.bySession, primarySessionId);
    }
    return (
      primarySessionState === "WAITING_FOR_INPUT" &&
      primarySessionPendingAction === "clarification"
    );
  });
}
