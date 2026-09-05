import type { TaskSession } from "@/lib/types/http";
import { sortSessions } from "@/components/task/session-sort";

export type ThreadSessionSelectionOptions = {
  /** Session requested by the Threads URL, if the task owns it. */
  requestedSessionId?: string | null;
  /** Session already selected by this task column. */
  currentSessionId?: string | null;
};

function firstPendingSession(
  sessions: readonly TaskSession[],
  pendingAction: TaskSession["pending_action"],
): TaskSession | undefined {
  return sessions.find((session) => session.pending_action === pendingAction);
}

/**
 * Selects the conversation that a task column should show.
 *
 * The current selection is checked before attention and activity so a status
 * update in a sibling session never steals the reader's conversation. The
 * caller can pass a URL selection only during the column's initial resolve.
 */
export function selectThreadSessionId(
  sessions: readonly TaskSession[],
  options: ThreadSessionSelectionOptions = {},
): string | null {
  if (sessions.length === 0) return null;
  const sessionIds = new Set(sessions.map((session) => String(session.id)));
  if (options.requestedSessionId && sessionIds.has(options.requestedSessionId)) {
    return options.requestedSessionId;
  }
  if (options.currentSessionId && sessionIds.has(options.currentSessionId)) {
    return options.currentSessionId;
  }

  const sorted = sortSessions(sessions);
  const permission = firstPendingSession(sorted, "permission");
  if (permission) return permission.id;
  const clarification = firstPendingSession(sorted, "clarification");
  if (clarification) return clarification.id;

  const active = sorted.find(
    (session) => session.state === "STARTING" || session.state === "RUNNING",
  );
  if (active) return active.id;

  const primary = sorted.find((session) => session.is_primary);
  return primary?.id ?? sorted[0]?.id ?? null;
}

/** Alias that reads naturally at call sites resolving a replacement choice. */
export const resolveThreadSessionId = selectThreadSessionId;
