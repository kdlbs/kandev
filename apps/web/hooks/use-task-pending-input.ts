import { useAppStore } from "@/components/state-provider";
import type { Message, TaskPendingAction, TaskSession } from "@/lib/types/http";
import {
  hasPendingClarificationForSession,
  hasPendingPermissionForSession,
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
  sessionId: string,
): PendingInput {
  return {
    clarification: hasPendingClarificationForSession(messagesBySession, sessionId),
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
        fallback?.taskId ? state.taskSessionsByTask.itemsByTaskId[fallback.taskId] : undefined,
        primarySessionId,
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
  taskSessions: TaskSession[],
  fallback: PendingInputFallback | undefined,
): PendingInput {
  const result = aggregateTaskPendingInput(
    taskSessions,
    (session) => {
      if (messagesBySession[session.id] === undefined) return undefined;
      return loadedSessionFlags(messagesBySession, session.id);
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
  primarySessionId: string | null | undefined,
  fallback: PendingInputFallback | undefined,
): PendingInput {
  const taskSnapshot = actionFlags(fallback?.taskPendingAction);
  if (taskSnapshot.clarification || taskSnapshot.permission) return taskSnapshot;
  if (!primarySessionId) return NONE;
  if (messagesBySession[primarySessionId] !== undefined) {
    return loadedSessionFlags(messagesBySession, primarySessionId);
  }
  return {
    clarification: fallbackFlag(fallback, "clarification"),
    permission: fallbackFlag(fallback, "permission"),
  };
}

function selectTaskPendingFlags(
  messagesBySession: Record<string, Message[] | undefined>,
  taskSessions: TaskSession[] | undefined,
  primarySessionId: string | null | undefined,
  fallback: PendingInputFallback | undefined,
): PendingInput {
  if (taskSessions?.length) {
    return selectFlagsFromLoadedTaskSessions(messagesBySession, taskSessions, fallback);
  }
  return selectFlagsFromPrimarySession(messagesBySession, primarySessionId, fallback);
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
