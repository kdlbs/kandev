import type { DockviewApi } from "dockview-react";
import { useDockviewStore } from "@/lib/state/dockview-store";
import {
  getEnvHiddenSessions,
  getEnvHiddenSessionRecords,
  setEnvHiddenSessions,
  setEnvHiddenSessionOwner,
} from "@/lib/env-hidden-sessions";
import type { TaskId } from "@/lib/types/http";

/**
 * Per-Dockview-API, per-environment memory of session panels the user
 * explicitly closed. The persisted record (env-keyed session storage) is the
 * authority across reloads; this module keeps the live set for the mounted
 * API and the write-through mutations.
 */

const hiddenSessionIdsByApi = new WeakMap<DockviewApi, Map<string | null, Set<string>>>();

/**
 * Persist the env's hidden set so a reload restores the user's explicit close
 * choices. The saved Dockview layout cannot express them: reusable layouts
 * collapse session panels into a chat placeholder, and restore paths add every
 * current session back as sibling panels.
 */
function persistHiddenSessionIds(envId: string | null, hiddenSessionIds: Set<string>): void {
  if (envId !== null) setEnvHiddenSessions(envId, [...hiddenSessionIds]);
}

/** The env's hidden set for this API, rehydrated from the persisted record. */
export function hiddenSessionIdsFor(api: DockviewApi): Set<string> {
  const envId = useDockviewStore.getState().currentLayoutEnvId;
  let hiddenSessionIdsByEnv = hiddenSessionIdsByApi.get(api);
  if (!hiddenSessionIdsByEnv) {
    hiddenSessionIdsByEnv = new Map();
    hiddenSessionIdsByApi.set(api, hiddenSessionIdsByEnv);
  }
  const existing = hiddenSessionIdsByEnv.get(envId);
  if (existing) return existing;
  // Seed from the persisted record so a fresh Dockview API after reload keeps
  // honoring panels the user closed before the reload.
  const hiddenSessionIds = new Set(envId === null ? [] : getEnvHiddenSessions(envId));
  hiddenSessionIdsByEnv.set(envId, hiddenSessionIds);
  return hiddenSessionIds;
}

/** True when the effective session's panel was explicitly closed and hidden. */
export function isHiddenSession(
  effectiveSessionId: string,
  hiddenSessionIds: Set<string>,
): boolean {
  return hiddenSessionIds.has(effectiveSessionId);
}

/**
 * Retire hidden records whose owning task's authoritative session list no
 * longer contains them. A shared environment spans tasks, so only the owning
 * task's list can retire its record; an owner whose list has not loaded yet
 * cannot be judged.
 */
export function pruneHiddenSessionIds(
  envId: string | null,
  hiddenSessionIds: Set<string>,
  store: () => {
    taskSessionsByTask?: {
      itemsByTaskId?: Record<string, Array<{ id: string }>>;
      loadedByTaskId?: Record<string, boolean>;
    };
    taskSessions?: { items?: Record<string, { task_id?: TaskId }> };
  },
): void {
  const state = store();
  const { itemsByTaskId, loadedByTaskId } = state.taskSessionsByTask ?? {};
  const sessionItems = state.taskSessions?.items ?? {};
  const persistedRecords = envId === null ? [] : getEnvHiddenSessionRecords(envId);
  const sessionOwnerIds = new Map(
    persistedRecords.map((record) => [record.sessionId, record.taskId]),
  );
  let pruned = false;
  for (const hiddenSessionId of hiddenSessionIds) {
    const ownerTaskId = ownerTaskIdFor(hiddenSessionId, sessionOwnerIds, sessionItems);
    if (!retireHiddenRecord(hiddenSessionId, ownerTaskId, itemsByTaskId, loadedByTaskId)) continue;
    hiddenSessionIds.delete(hiddenSessionId);
    pruned = true;
  }
  if (pruned) persistHiddenSessionIds(envId, hiddenSessionIds);
}

/**
 * Resolve the task that owns a hidden session: prefer the attribution captured
 * at hide time, then the store's session row (the reload path may not have
 * re-persisted the attribution yet).
 */
function ownerTaskIdFor(
  hiddenSessionId: string,
  sessionOwnerIds: Map<string, string>,
  sessionItems: Record<string, { task_id?: TaskId }>,
): string {
  return sessionOwnerIds.get(hiddenSessionId) || (sessionItems[hiddenSessionId]?.task_id ?? "");
}

function retireHiddenRecord(
  hiddenRecordId: string,
  ownerTaskId: string,
  itemsByTaskId: Record<string, Array<{ id: string }>> | undefined,
  loadedByTaskId: Record<string, boolean> | undefined,
): boolean {
  if (!ownerTaskId || !itemsByTaskId || !loadedByTaskId) return false;
  const ownerSessions = itemsByTaskId[ownerTaskId];
  if (!ownerSessions || loadedByTaskId[ownerTaskId] !== true) return false;
  return !ownerSessions.some((session) => session.id === hiddenRecordId);
}

/**
 * Hide a session tab without changing its persisted session lifecycle.
 *
 * The owning task is recorded so pruning can wait for that task's own
 * authoritative session list; a shared environment spans tasks, and any
 * single task's list cannot retire another task's record.
 */
export function hideSessionPanel(api: DockviewApi, sessionId: string, taskId?: string): void {
  const envId = useDockviewStore.getState().currentLayoutEnvId;
  if (envId !== null && taskId) setEnvHiddenSessionOwner(envId, sessionId, taskId);
  const hiddenSessionIds = hiddenSessionIdsFor(api);
  hiddenSessionIds.add(sessionId);
  persistHiddenSessionIds(envId, hiddenSessionIds);
  const panel = api.getPanel(`session:${sessionId}`);
  if (panel) api.removePanel(panel);
}

/** Forget a prior hide when a session is reopened or deleted. */
export function clearHiddenSessionPanel(api: DockviewApi, sessionId: string): void {
  const envId = useDockviewStore.getState().currentLayoutEnvId;
  const hiddenSessionIds = hiddenSessionIdsFor(api);
  hiddenSessionIds.delete(sessionId);
  persistHiddenSessionIds(envId, hiddenSessionIds);
}
