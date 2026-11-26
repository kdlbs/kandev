import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { TaskSession, TaskSessionState } from "@/lib/types/http";
import { parseTurnTimestamp } from "@/lib/state/slices/session/turn-actions";
import { isConfirmedSessionRemoved } from "@/lib/state/session-removal";

// i18n-exempt: Internal follow-source discriminant, never rendered.
export const NEW_SESSION_SOURCE = "new-session";
export type FollowSource = "primary" | "terminal" | "new-session" | "removed";

export function allowsFollowState(source: FollowSource, state: TaskSession["state"]): boolean {
  if (!state || state === "FAILED" || state === "STARTING") return false;
  return (
    source === "primary" ||
    state === "COMPLETED" ||
    state === "CANCELLED" ||
    (source === NEW_SESSION_SOURCE && state === "IDLE")
  );
}

export function hasExplicitWorkflowFocus(state: AppState, taskId: string): boolean {
  const focus = state.workflowSessionFocus;
  return focus?.intent?.taskId === taskId || focus?.request?.taskId === taskId;
}

export function isUnavailableSession(
  state: AppState,
  sessionId: string | null | undefined,
  observed?: ReadonlySet<string>,
): boolean {
  return (
    !!sessionId &&
    (observed?.has(sessionId) || isConfirmedSessionRemoved(state.taskSessions, sessionId))
  );
}

const TERMINAL_SESSION_STATES: ReadonlySet<TaskSessionState> = new Set([
  "COMPLETED",
  "CANCELLED",
  "FAILED",
]);

export function isTerminalSessionState(state: TaskSessionState | undefined): boolean {
  return !!state && TERMINAL_SESSION_STATES.has(state);
}

export function hasSameTaskPredecessor(
  state: AppState,
  taskId: string,
  previous: string | null,
): previous is string {
  const row = previous ? state.taskSessions.items[previous] : undefined;
  return !!previous && (!row || row.task_id === taskId);
}

export function isEligibleReplacementSession(
  row: TaskSession | undefined,
  taskId: string,
  excluded?: ReadonlySet<string>,
): row is TaskSession {
  return (
    !!row &&
    row.task_id === taskId &&
    !!row.state &&
    !isTerminalSessionState(row.state) &&
    !excluded?.has(row.id)
  );
}

function isNewerSession(
  id: string,
  time: bigint | null,
  newestId: string | undefined,
  newestTime: bigint | null,
): boolean {
  return (
    !newestId ||
    (time !== null && (newestTime === null || time > newestTime)) ||
    (time === newestTime && id < newestId)
  );
}

/** Newest currently eligible same-task lifecycle recipient. */
export function pickReplacementSessionId(
  state: AppState,
  taskId: string,
  excluded?: ReadonlySet<string>,
): string | null | undefined {
  const sessions = state.taskSessionsByTask?.itemsByTaskId[taskId];
  let newest: TaskSession | undefined;
  let newestTime: bigint | null = null;
  let eligible = 0;
  let unknownTime = false;
  for (const candidate of sessions ?? []) {
    const row = state.taskSessions?.items[candidate.id] ?? candidate;
    if (!isEligibleReplacementSession(row, taskId, excluded)) continue;
    if (isConfirmedSessionRemoved(state.taskSessions, row.id)) continue;
    eligible += 1;
    const time = parseTurnTimestamp(row.started_at);
    unknownTime ||= time === null;
    if (isNewerSession(row.id, time, newest?.id, newestTime)) {
      newest = row;
      newestTime = time;
    }
  }
  if (eligible > 1 && unknownTime) return undefined;
  return newest?.id ?? null;
}

function findSessionForTask(state: AppState, taskId: string, sessionId: string) {
  const byTask = state.taskSessionsByTask;
  const sessionsForTask = byTask?.itemsByTaskId?.[taskId];
  if (byTask?.loadedByTaskId?.[taskId]) {
    return sessionsForTask?.find((session) => session.id === sessionId) ?? null;
  }
  // Some task-update call sites use partial stores before taskSessions hydrates.
  const byId = state.taskSessions?.items?.[sessionId];
  if (byId) return byId.task_id === taskId ? byId : null;
  return sessionsForTask?.find((session) => session.id === sessionId) ?? null;
}

function isTaskSessionListHydrating(state: AppState, taskId: string): boolean {
  const byTask = state.taskSessionsByTask;
  if (!byTask) return true;
  if (byTask.loadingByTaskId?.[taskId]) return true;
  return !byTask.loadedByTaskId?.[taskId];
}

/**
 * Manual session selection pins a task-scoped session. Background WS events
 * may only override that pin once the pinned session is known terminal, or
 * when the terminal event is for the pinned session itself.
 */
export function shouldPreservePinnedSessionForTask(
  state: AppState,
  taskId: string,
  incoming?: { sessionId: string; newState: TaskSessionState | undefined },
): boolean {
  const pinnedSessionId = state.tasks.pinnedSessionId;
  if (!pinnedSessionId || state.tasks.activeTaskId !== taskId) return false;
  if (
    incoming?.sessionId === pinnedSessionId &&
    incoming.newState &&
    isTerminalSessionState(incoming.newState)
  ) {
    return false;
  }

  const pinnedSession = findSessionForTask(state, taskId, pinnedSessionId);
  if (!pinnedSession) {
    // Preserve missing rows only while the per-task list is still hydrating.
    // Once loaded, absence means the pinned session was deleted or went stale.
    return isTaskSessionListHydrating(state, taskId);
  }
  return !isTerminalSessionState(pinnedSession.state);
}

export function clearPinnedSessionIfOverridden(store: StoreApi<AppState>, sessionId: string): void {
  const pinnedSessionId = store.getState().tasks.pinnedSessionId;
  if (!pinnedSessionId || pinnedSessionId === sessionId) return;
  store.setState((state) => ({
    ...state,
    tasks: { ...state.tasks, pinnedSessionId: null },
  }));
}

export function inheritFallbackReadiness(
  state: AppState,
  predecessor: string,
  target: string,
): void {
  const previous = state.sessionAgentctl.itemsBySessionId[predecessor];
  const current = state.sessionAgentctl.itemsBySessionId[target];
  const executionId = state.taskSessions.items[target]?.agent_execution_id;
  if (
    previous?.status !== "ready" ||
    current ||
    (executionId && executionId !== previous.agentExecutionId)
  )
    return;
  const targetEnvironment =
    state.environmentIdBySessionId?.[target] ??
    state.taskSessions.items[target]?.task_environment_id;
  const previousEnvironment =
    state.environmentIdBySessionId?.[predecessor] ??
    state.taskSessions.items[predecessor]?.task_environment_id;
  if (targetEnvironment && targetEnvironment !== previousEnvironment) return;
  state.setSessionAgentctlStatus(target, {
    status: "ready",
    agentExecutionId: executionId,
    updatedAt: previous.updatedAt,
  });
}

export function resolveLifecycleTarget(
  state: AppState,
  pending: {
    taskId: string;
    previousSessionId: string;
    projectedPrimaryId?: string | null;
    targetSessionId?: string | null;
    source: FollowSource;
  },
  excluded?: ReadonlySet<string>,
): string | null {
  const primary = pending.projectedPrimaryId;
  const projected = primary ? state.taskSessions.items[primary] : undefined;
  const unavailable = isUnavailableSession(state, primary, excluded);
  if (primary && primary !== pending.previousSessionId && !projected && !unavailable) return null;
  const hinted = pending.targetSessionId
    ? state.taskSessions.items[pending.targetSessionId]
    : undefined;
  if (
    pending.source === NEW_SESSION_SOURCE &&
    isEligibleReplacementSession(hinted, pending.taskId, excluded)
  ) {
    if (!isConfirmedSessionRemoved(state.taskSessions, hinted.id)) return hinted.id;
  }
  const ranked = pickReplacementSessionId(state, pending.taskId, excluded);
  if (ranked === undefined) return null;
  if (ranked) return ranked;
  return isEligibleReplacementSession(projected, pending.taskId, excluded)
    ? (primary ?? null)
    : null;
}
