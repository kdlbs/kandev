import { useAppStore } from "@/components/state-provider";
import type { Message, TaskPendingAction, TaskSession, Turn } from "@/lib/types/http";
import {
  hasPendingClarificationForSession,
  hasPendingPermissionForSession,
  newestDurableTurnId,
  type PendingClarificationScope,
} from "@/lib/utils/pending-clarification";
import { aggregateTaskPendingInput } from "@/lib/utils/task-pending-input";

export type PendingInput = { clarification: boolean; permission: boolean };

const NONE: PendingInput = { clarification: false, permission: false };

export type PendingInputFallback = {
  taskId?: string | null;
  taskPendingAction?: TaskPendingAction | null;
  primarySessionState?: string | null;
  primarySessionPendingAction?: TaskPendingAction | null;
};

type PrimarySessionProjection = {
  id: string | null | undefined;
  pendingAction: TaskPendingAction | null | undefined;
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

function actionFlags(action: TaskPendingAction | null | undefined): PendingInput {
  return { clarification: action === "clarification", permission: action === "permission" };
}

function loadedSessionFlags(
  messagesBySession: Record<string, Message[] | undefined>,
  turnsBySession: Record<string, readonly Turn[] | undefined>,
  sessionId: string,
  pendingAction?: TaskPendingAction | null,
): PendingInput {
  const currentTurnId = newestDurableTurnId(turnsBySession[sessionId]);
  // With both authority signals unavailable, preserve the legacy full-message
  // scan. Once either loads, explicit turn/action scoping applies.
  const clarificationScope: PendingClarificationScope | undefined =
    currentTurnId === undefined && pendingAction === undefined
      ? undefined
      : { currentTurnId, pendingAction };
  return {
    clarification: hasPendingClarificationForSession(
      messagesBySession,
      sessionId,
      clarificationScope,
    ),
    permission: hasPendingPermissionForSession(messagesBySession, sessionId),
  };
}

/**
 * Task-level pending-input flags across every input-capable session. Prefers
 * loaded per-session messages and uses the task-wide boot snapshot for sessions
 * whose messages are not loaded yet. The primary-session fallback remains for
 * compatibility with older payloads.
 *
 */
export function useTaskPendingInput(
  primarySessionId: string | null | undefined,
  fallback?: PendingInputFallback,
): PendingInput {
  // Encode the two booleans as a primitive bitmask so the zustand selector
  // returns a value stable under `Object.is` — a fresh object every render
  // would defeat useSyncExternalStore's snapshot caching and loop forever.
  const flags = useAppStore((state) =>
    toBitmask(
      selectTaskPendingFlags(
        state.messages.bySession,
        state.turns.bySession,
        fallback?.taskId ? state.taskSessionsByTask.itemsByTaskId[fallback.taskId] : undefined,
        {
          id: primarySessionId,
          pendingAction: primarySessionId
            ? state.taskSessions.items[primarySessionId]?.pending_action
            : undefined,
        },
        fallback,
      ),
    ),
  );
  return { clarification: (flags & 1) !== 0, permission: (flags & 2) !== 0 };
}

function toBitmask(flags: PendingInput): number {
  return (flags.clarification ? 1 : 0) | (flags.permission ? 2 : 0);
}

function selectFlagsFromLoadedTaskSessions(
  messagesBySession: Record<string, Message[] | undefined>,
  turnsBySession: Record<string, readonly Turn[] | undefined>,
  taskSessions: TaskSession[],
  fallback: PendingInputFallback | undefined,
): PendingInput {
  const result = aggregateTaskPendingInput(
    taskSessions,
    (session) => {
      if (messagesBySession[session.id] === undefined) return undefined;
      return loadedSessionFlags(
        messagesBySession,
        turnsBySession,
        session.id,
        session.pending_action,
      );
    },
    fallback?.taskPendingAction,
  );
  if (!result.hasUnloadedMessages) {
    return { clarification: result.clarification, permission: result.permission };
  }
  return {
    clarification: result.clarification || fallbackFlag(fallback, "clarification"),
    permission: result.permission || fallbackFlag(fallback, "permission"),
  };
}

function selectFlagsFromPrimarySession(
  messagesBySession: Record<string, Message[] | undefined>,
  turnsBySession: Record<string, readonly Turn[] | undefined>,
  primarySessionId: string | null | undefined,
  primarySessionPendingAction: TaskPendingAction | null | undefined,
  fallback: PendingInputFallback | undefined,
): PendingInput {
  const taskSnapshot = actionFlags(fallback?.taskPendingAction);
  if (taskSnapshot.clarification || taskSnapshot.permission) return taskSnapshot;
  if (!primarySessionId) return NONE;
  if (messagesBySession[primarySessionId] !== undefined) {
    return loadedSessionFlags(
      messagesBySession,
      turnsBySession,
      primarySessionId,
      primarySessionPendingAction !== undefined
        ? primarySessionPendingAction
        : fallback?.primarySessionPendingAction,
    );
  }
  return {
    clarification: fallbackFlag(fallback, "clarification"),
    permission: fallbackFlag(fallback, "permission"),
  };
}

function selectTaskPendingFlags(
  messagesBySession: Record<string, Message[] | undefined>,
  turnsBySession: Record<string, readonly Turn[] | undefined>,
  taskSessions: TaskSession[] | undefined,
  primarySession: PrimarySessionProjection,
  fallback: PendingInputFallback | undefined,
): PendingInput {
  if (taskSessions?.length) {
    return selectFlagsFromLoadedTaskSessions(
      messagesBySession,
      turnsBySession,
      taskSessions,
      fallback,
    );
  }
  return selectFlagsFromPrimarySession(
    messagesBySession,
    turnsBySession,
    primarySession.id,
    primarySession.pendingAction,
    fallback,
  );
}

/**
 * Per-session pending-input flags. Loaded messages are authoritative; when a
 * transcript is not hydrated, use the compact session projection from boot or
 * the session list instead.
 */
export function useSessionPendingInput(sessionId: string | null | undefined): PendingInput {
  const flags = useAppStore((state) => {
    if (!sessionId) return 0;
    if (state.messages.bySession[sessionId] !== undefined) {
      return toBitmask(
        loadedSessionFlags(
          state.messages.bySession,
          state.turns.bySession,
          sessionId,
          state.taskSessions?.items?.[sessionId]?.pending_action,
        ),
      );
    }
    return toBitmask(actionFlags(state.taskSessions?.items?.[sessionId]?.pending_action));
  });
  return { clarification: (flags & 1) !== 0, permission: (flags & 2) !== 0 };
}
