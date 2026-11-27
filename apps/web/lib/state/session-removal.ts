import { getBackendConfig } from "@/lib/config";
import type { TaskSession, TaskSessionsResponse } from "@/lib/types/http";
import type { TaskSessionsState } from "./slices/session/types";

export type ConfirmedSessionRemovals = {
  backend: string;
  ids: Record<string, true>;
};

function removedIds(state: TaskSessionsState): Readonly<Record<string, true>> | undefined {
  const removed = state.confirmedRemovals;
  return removed && removed.backend === getBackendConfig().apiBaseUrl ? removed.ids : undefined;
}

/** A new queue incarnation is not proof that a deleted session UUID exists again. */
export function isConfirmedSessionRemoved(state: TaskSessionsState, sessionId: string): boolean {
  return removedIds(state)?.[sessionId] === true;
}

const MAX_CONFIRMED_REMOVALS = 100;

export function recordConfirmedSessionRemoval(state: TaskSessionsState, sessionId: string): void {
  const backend = getBackendConfig().apiBaseUrl;
  let removed = state.confirmedRemovals;
  if (!removed || removed.backend !== backend) {
    removed = { backend, ids: {} };
    state.confirmedRemovals = removed;
  }
  const keys = Object.keys(removed.ids);
  if (!removed.ids[sessionId] && keys.length >= MAX_CONFIRMED_REMOVALS) {
    const oldest = keys[0];
    if (oldest) delete removed.ids[oldest];
  }
  removed.ids[sessionId] = true;
}

/** A stale membership cannot discard surviving rows learned after its snapshot. */
export function admitTaskSessionRows(
  state: TaskSessionsState,
  existing: TaskSession[],
  incoming: TaskSession[],
): TaskSession[] {
  const removed = removedIds(state);
  if (!removed || !incoming.some((row) => removed[row.id])) return incoming;
  const rows = new Map(existing.filter((row) => !removed[row.id]).map((row) => [row.id, row]));
  for (const row of incoming) {
    if (!removed[row.id]) rows.set(row.id, row);
  }
  return [...rows.values()];
}

/** Only complete, unique, same-task absence authorizes retirement or a successor. */
export function proveRemovedSessionMembership(
  membership: TaskSessionsResponse,
  taskId: string,
  removedSessionId: string,
): { primary: TaskSession | null } | null {
  if (membership.total !== membership.sessions.length) return null;
  let primary: TaskSession | null = null;
  const ids = new Set<string>();
  for (const session of membership.sessions) {
    if (
      !session.id ||
      ids.has(session.id) ||
      session.task_id !== taskId ||
      session.id === removedSessionId
    )
      return null;
    ids.add(session.id);
    if (session.is_primary) {
      if (primary) return null;
      primary = session;
    }
  }
  return { primary };
}

/** Complete, unique, same-task membership that still lists the session disproves its removal. */
export function disprovesSessionRemoval(
  membership: TaskSessionsResponse,
  taskId: string,
  sessionId: string,
): boolean {
  if (membership.total !== membership.sessions.length) return false;
  const ids = new Set<string>();
  for (const session of membership.sessions) {
    if (!session.id || ids.has(session.id) || session.task_id !== taskId) return false;
    ids.add(session.id);
  }
  return ids.has(sessionId);
}
