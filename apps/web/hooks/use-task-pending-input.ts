import { useAppStore } from "@/components/state-provider";
import type { Message, TaskPendingAction, TaskSession, TaskSessionState } from "@/lib/types/http";
import {
  hasPendingClarificationForSession,
  hasPendingPermissionForSession,
} from "@/lib/utils/pending-clarification";

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
  const flags = useAppStore((state) =>
    selectTaskPendingFlags(
      state.messages.bySession,
      fallback?.taskId ? state.taskSessionsByTask.itemsByTaskId[fallback.taskId] : undefined,
      primarySessionId,
      fallback,
    ),
  );
  return { clarification: (flags & 1) !== 0, permission: (flags & 2) !== 0 };
}

function selectTaskPendingFlags(
  messagesBySession: Record<string, Message[] | undefined>,
  taskSessions: TaskSession[] | undefined,
  primarySessionId: string | null | undefined,
  fallback: PendingInputFallback | undefined,
): number {
  if (taskSessions?.length) {
    const live = loadedTaskPendingFlags(messagesBySession, taskSessions);
    return live.hasUnloadedMessages
      ? live.flags |
          actionFlag(fallback?.taskPendingAction) |
          (fallbackFlag(fallback, "permission") ? 2 : 0) |
          (fallbackFlag(fallback, "clarification") ? 1 : 0)
      : live.flags;
  }
  const taskSnapshot = actionFlag(fallback?.taskPendingAction);
  if (taskSnapshot) return taskSnapshot;
  if (!primarySessionId) return 0;
  if (messagesBySession[primarySessionId] !== undefined) {
    return loadedSessionFlags(messagesBySession, primarySessionId);
  }
  return (
    (fallbackFlag(fallback, "permission") ? 2 : 0) |
    (fallbackFlag(fallback, "clarification") ? 1 : 0)
  );
}

function loadedTaskPendingFlags(
  messagesBySession: Record<string, Message[] | undefined>,
  sessions: TaskSession[],
): { flags: number; hasUnloadedMessages: boolean } {
  let flags = 0;
  let hasUnloadedMessages = false;
  for (const session of sessions) {
    if (!isInputCapable(session.state)) continue;
    if (messagesBySession[session.id] === undefined) {
      hasUnloadedMessages = true;
      continue;
    }
    flags |= loadedSessionFlags(messagesBySession, session.id);
  }
  return { flags, hasUnloadedMessages };
}

function loadedSessionFlags(
  messagesBySession: Record<string, Message[] | undefined>,
  sessionId: string,
): number {
  return (
    (hasPendingPermissionForSession(messagesBySession, sessionId) ? 2 : 0) |
    (hasPendingClarificationForSession(messagesBySession, sessionId) ? 1 : 0)
  );
}

function actionFlag(action: TaskPendingAction | null | undefined): number {
  if (action === "permission") return 2;
  if (action === "clarification") return 1;
  return 0;
}

function isInputCapable(state: TaskSessionState): boolean {
  return state === "RUNNING" || state === "WAITING_FOR_INPUT";
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
