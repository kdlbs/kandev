import { useAppStore } from "@/components/state-provider";
import type { TaskPendingAction } from "@/lib/types/http";
import {
  hasPendingClarificationForSession,
  hasPendingPermissionForSession,
} from "@/lib/utils/pending-clarification";

export type PendingInput = { clarification: boolean; permission: boolean };

const NONE: PendingInput = { clarification: false, permission: false };

export type PendingInputFallback = {
  primarySessionState?: string | null;
  primarySessionPendingAction?: TaskPendingAction | null;
};

function fallbackFlag(
  fallback: PendingInputFallback | undefined,
  action: TaskPendingAction,
): boolean {
  return (
    fallback?.primarySessionState === "WAITING_FOR_INPUT" &&
    fallback.primarySessionPendingAction === action
  );
}

/**
 * Task-level pending-input flags for the task's primary session. Prefers the
 * live per-session messages in the store; when those are not loaded yet it
 * falls back to the boot-payload snapshot (`primary_session_state` +
 * `primary_session_pending_action`) so the reading is correct on first paint.
 *
 */
export function useTaskPendingInput(
  primarySessionId: string | null | undefined,
  fallback?: PendingInputFallback,
): PendingInput {
  const clarification = useAppStore((state) => {
    if (!primarySessionId) return false;
    const messages = state.messages.bySession[primarySessionId];
    if (messages !== undefined)
      return hasPendingClarificationForSession(state.messages.bySession, primarySessionId);
    return fallbackFlag(fallback, "clarification");
  });
  const permission = useAppStore((state) => {
    if (!primarySessionId) return false;
    const messages = state.messages.bySession[primarySessionId];
    if (messages !== undefined)
      return hasPendingPermissionForSession(state.messages.bySession, primarySessionId);
    return fallbackFlag(fallback, "permission");
  });
  return { clarification, permission };
}

/** Per-session pending-input flags; session menus already operate on loaded sessions. */
export function useSessionPendingInput(sessionId: string | null | undefined): PendingInput {
  const clarification = useAppStore((state) =>
    hasPendingClarificationForSession(state.messages.bySession, sessionId),
  );
  const permission = useAppStore((state) =>
    hasPendingPermissionForSession(state.messages.bySession, sessionId),
  );
  if (!sessionId) return NONE;
  return { clarification, permission };
}
