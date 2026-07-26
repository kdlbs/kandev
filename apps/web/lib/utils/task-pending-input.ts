import type { TaskPendingAction } from "@/lib/types/http";

export type PendingInputFlags = { clarification: boolean; permission: boolean };

/**
 * Session states where the session can request input from the user. Used to
 * decide which sessions participate in the task-level pending-input
 * aggregate — a COMPLETED/FAILED/CANCELLED session's stale message history
 * must never light up a task-level clarification/permission indicator.
 */
export function isInputCapableSessionState(state: string): boolean {
  return state === "RUNNING" || state === "WAITING_FOR_INPUT";
}

export type AggregatedPendingInput = PendingInputFlags & { hasUnloadedMessages: boolean };

/**
 * Shared task-level pending-input aggregation: OR together the
 * clarification/permission flags of every input-capable session, falling
 * back to `taskPendingAction` when a session's messages are not loaded yet
 * (or when there are no sessions at all) — the task-wide boot snapshot is the
 * best available signal until the per-session messages arrive.
 *
 * `getSessionFlags` returns `undefined` for a session whose messages are not
 * loaded, and the loaded `{ clarification, permission }` flags otherwise.
 * Callers that need extra legacy fallback layers (e.g. a primary-session
 * substate check) can inspect `hasUnloadedMessages` on the result and layer
 * additional fallback on top.
 */
export function aggregateTaskPendingInput<S extends { state: string }>(
  sessions: readonly S[],
  getSessionFlags: (session: S) => PendingInputFlags | undefined,
  taskPendingAction: TaskPendingAction | null | undefined,
): AggregatedPendingInput {
  let clarification = false;
  let permission = false;
  let hasUnloadedMessages = false;
  for (const session of sessions) {
    if (!isInputCapableSessionState(session.state)) continue;
    const flags = getSessionFlags(session);
    if (flags === undefined) {
      hasUnloadedMessages = true;
      continue;
    }
    clarification ||= flags.clarification;
    permission ||= flags.permission;
  }
  if (sessions.length === 0 || hasUnloadedMessages) {
    clarification ||= taskPendingAction === "clarification";
    permission ||= taskPendingAction === "permission";
  }
  return { clarification, permission, hasUnloadedMessages };
}
