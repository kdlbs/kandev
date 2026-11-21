import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { fetchTaskSession, listTaskSessions } from "@/lib/api/domains/session-api";
import { ApiError } from "@/lib/api/client";
import type { TaskSession, TaskSessionsResponse } from "@/lib/types/http";
import { retireSessionReads } from "@/lib/state/session-read-coordinator";
import {
  getTaskSessionReads,
  invalidateTaskSessionMembership,
} from "@/lib/state/task-session-reads";
import { isConfirmedSessionRemoved } from "@/lib/state/session-removal";
import {
  observedRemovals,
  recordObservedSessionRemoval,
} from "@/lib/state/session-removal-observations";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import {
  getProvenRemovalPredecessor,
  recordRemovalHandoff,
} from "@/lib/state/session-removal-handoff";
import { createDebugLogger } from "@/lib/debug/log";
import {
  getSessionRestartStore,
  isSessionRestartPending,
} from "@/lib/session/session-restart-state";
import {
  clearPinnedSessionIfOverridden,
  hasSameTaskPredecessor,
  isEligibleReplacementSession,
  hasExplicitWorkflowFocus,
  isUnavailableSession,
  pickReplacementSessionId,
  shouldPreservePinnedSessionForTask,
  allowsFollowState,
  NEW_SESSION_SOURCE,
  inheritFallbackReadiness,
  type FollowSource,
} from "./session-selection";
import type { KanbanTask } from "./task-archive-cache";

const debug = createDebugLogger("task-lifecycle:ws");
type FollowOwner = {
  taskId: string;
  previousSessionId: string;
  selectedSessionId: string | null;
  targetSessionId: string | null;
  projectedPrimaryId: string | null | undefined;
  source: FollowSource;
  lifecycleRecipient: boolean;
  scopeIdentity: string;
  navigationRevision: number | undefined;
  pinnedSessionId: string | null;
  workflowFocus: AppState["workflowSessionFocus"];
};
type PendingFollow = FollowOwner & { cancel: () => void; authorized: boolean };
type RemovalOwner = Pick<FollowOwner, "taskId" | "previousSessionId" | "scopeIdentity">;
type RemovalTarget = Pick<TaskSession, "id" | "task_id" | "state" | "started_at" | "updated_at">;
// Denied or uncertain reads retain view ownership until selection changes.
const pendingByStore = new WeakMap<StoreApi<AppState>, PendingFollow>();

function observeRemoval(store: StoreApi<AppState>, taskId: string, sessionId: string): void {
  if (recordObservedSessionRemoval(store, sessionId))
    invalidateTaskSessionMembership(store, taskId);
}

/** Look up a task across both single-kanban and multi-kanban snapshots. */
export function findTaskInState(state: AppState, taskId: string): KanbanTask | undefined {
  const fromKanban = state.kanban.tasks.find((task) => task.id === taskId);
  if (fromKanban) return fromKanban;
  for (const snapshot of Object.values(state.kanbanMulti.snapshots)) {
    const found = snapshot.tasks.find((task) => task.id === taskId);
    if (found) return found;
  }
  return undefined;
}

function ownsSelection(store: StoreApi<AppState>, pending: FollowOwner): boolean {
  const state = store.getState();
  return (
    stateReadScopeIdentity(state) === pending.scopeIdentity &&
    state.tasks.activeTaskId === pending.taskId &&
    state.tasks.activeSessionId === pending.selectedSessionId &&
    state.tasks.pinnedSessionId === pending.pinnedSessionId &&
    state.taskRemoval?.navigationRevision === pending.navigationRevision &&
    state.workflowSessionFocus === pending.workflowFocus &&
    !hasExplicitWorkflowFocus(state, pending.taskId) &&
    !isSessionRestartPending(store, pending.previousSessionId) &&
    (!shouldPreservePinnedSessionForTask(state, pending.taskId) ||
      (pending.source === "removed" &&
        state.tasks.pinnedSessionId === pending.previousSessionId)) &&
    (pending.source !== "primary" || !!findTaskInState(state, pending.taskId))
  );
}

function ownsPendingFollow(store: StoreApi<AppState>, pending: PendingFollow): boolean {
  return pendingByStore.get(store) === pending && ownsSelection(store, pending);
}

function lifecycleTarget(store: StoreApi<AppState>, pending: FollowOwner): string | null {
  const state = store.getState();
  const excluded = observedRemovals(store);
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

function prepareFollowTarget(
  store: StoreApi<AppState>,
  pending: FollowOwner,
  removedTarget?: RemovalTarget,
): string | null {
  const state = store.getState();
  const excluded = observedRemovals(store);
  const target =
    removedTarget?.id ??
    (pending.source === "terminal" || pending.source === NEW_SESSION_SOURCE
      ? lifecycleTarget(store, pending)
      : pending.targetSessionId);
  if (
    !target ||
    target === pending.previousSessionId ||
    isUnavailableSession(state, target, excluded)
  )
    return null;
  if (
    !removedTarget &&
    pending.lifecycleRecipient &&
    !isEligibleReplacementSession(state.taskSessions.items[target], pending.taskId, excluded)
  )
    return null;
  if (removedTarget && !state.taskSessions.items[target]) {
    state.upsertTaskSessionFromEvent(pending.taskId, {
      id: removedTarget.id,
      task_id: removedTarget.task_id,
      state: removedTarget.state,
      started_at: removedTarget.started_at,
      updated_at: removedTarget.updated_at,
    });
  }
  return target;
}

function completePendingFollow(
  store: StoreApi<AppState>,
  pending: PendingFollow,
  removedTarget?: RemovalTarget,
): boolean {
  if (!ownsPendingFollow(store, pending)) return false;
  const state = store.getState();
  if (findTaskInState(state, pending.taskId)?.primarySessionId !== pending.projectedPrimaryId)
    return false;
  const target = prepareFollowTarget(store, pending, removedTarget);
  if (!target) return false;
  recordRemovalHandoff(store, pending, target, removedTarget !== undefined);
  pending.cancel();
  if (!removedTarget && (pending.source === "terminal" || pending.source === NEW_SESSION_SOURCE)) {
    inheritFallbackReadiness(state, pending.previousSessionId, target);
  }
  clearPinnedSessionIfOverridden(store, target);
  store.getState().setActiveSessionAuto(pending.taskId, target);
  return true;
}

function provenRemoval(
  membership: TaskSessionsResponse,
  pending: RemovalOwner,
): { primary: RemovalTarget | null } | null {
  if (membership.total !== membership.sessions.length) return null;
  let primary: RemovalTarget | null = null;
  const ids = new Set<string>();
  for (const session of membership.sessions) {
    if (
      !session.id ||
      ids.has(session.id) ||
      session.task_id !== pending.taskId ||
      session.id === pending.previousSessionId
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

function retainEmptySelection(
  store: StoreApi<AppState>,
  removal: RemovalOwner,
  view?: PendingFollow,
): void {
  const state = store.getState();
  if (
    state.tasks.activeTaskId !== removal.taskId ||
    state.tasks.activeSessionId !== removal.previousSessionId
  )
    return;
  const pin =
    state.tasks.pinnedSessionId === removal.previousSessionId ? null : state.tasks.pinnedSessionId;
  if (view && ownsPendingFollow(store, view)) {
    view.selectedSessionId = null;
    view.pinnedSessionId = pin;
  }
  recordRemovalHandoff(
    store,
    captureOwner(state, removal.taskId, removal.previousSessionId, null, "removed"),
    null,
    true,
  );
  store.setState((current) => ({
    ...current,
    tasks: { ...current.tasks, activeSessionId: null, pinnedSessionId: pin },
  }));
}

function ownsEmptySelection(store: StoreApi<AppState>, removal: RemovalOwner): boolean {
  const view = pendingByStore.get(store);
  return (
    !!view &&
    view.previousSessionId === removal.previousSessionId &&
    view.selectedSessionId === null &&
    ownsPendingFollow(store, view)
  );
}

function retireRemovalMembership(store: StoreApi<AppState>, pending: RemovalOwner): void {
  const state = store.getState();
  if (state.workflowSessionFocus.request?.sessionId === pending.previousSessionId) {
    state.cancelWorkflowSessionFocus({ taskId: pending.taskId });
  }
  if (isConfirmedSessionRemoved(store.getState().taskSessions, pending.previousSessionId)) return;
  invalidateTaskSessionMembership(store, pending.taskId);
  retireSessionReads(store, pending.previousSessionId);
  store
    .getState()
    .removeTaskSession(pending.taskId, pending.previousSessionId, { confirmed: true });
  observedRemovals(store)?.delete(pending.previousSessionId);
}

function commitRemovalMembership(
  store: StoreApi<AppState>,
  pending: RemovalOwner,
  membership: TaskSessionsResponse,
) {
  const proof = provenRemoval(membership, pending);
  if (!proof) return;
  const view = pendingByStore.get(store);
  const matchingView =
    view?.taskId === pending.taskId && view.previousSessionId === pending.previousSessionId
      ? view
      : undefined;
  if (
    !proof.primary ||
    !matchingView ||
    !completePendingFollow(store, matchingView, proof.primary)
  ) {
    retainEmptySelection(store, pending, matchingView);
  }
  retireRemovalMembership(store, pending);
}

async function verifyRemoval(store: StoreApi<AppState>, pending: RemovalOwner) {
  if (stateReadScopeIdentity(store.getState()) !== pending.scopeIdentity) return;
  const reads = getTaskSessionReads(store);
  const release = reads.retain(pending.taskId);
  try {
    while (
      reads.isCurrent(pending.taskId) &&
      (!isConfirmedSessionRemoved(store.getState().taskSessions, pending.previousSessionId) ||
        ownsEmptySelection(store, pending))
    ) {
      const generation = reads.generation(pending.taskId);
      let membership: TaskSessionsResponse;
      try {
        membership = await listTaskSessions(pending.taskId, { cache: "no-store" });
      } catch (error) {
        if (reads.isCurrent(pending.taskId, generation)) throw error;
        continue;
      }
      if (!reads.isCurrent(pending.taskId, generation)) continue;
      commitRemovalMembership(store, pending, membership);
      return;
    }
  } finally {
    release();
  }
}

function reconcileAuthorizedFollow(store: StoreApi<AppState>, pending: PendingFollow): void {
  if (!pending.authorized) return;
  const cached = store.getState().taskSessions.items[pending.previousSessionId]?.state;
  if (cached && !allowsFollowState(pending.source, cached)) {
    pending.authorized = false;
    return;
  }
  completePendingFollow(store, pending);
}

function verifyPreviousSession(store: StoreApi<AppState>, owner: FollowOwner): void {
  const controller = new AbortController();
  const pending: PendingFollow = {
    ...owner,
    authorized: false,
    cancel: () => {
      controller.abort();
      finish();
    },
  };
  let observed = store.getState().taskSessions.items[pending.previousSessionId];
  let recheckQueued = false;
  const releaseView = store.subscribe(() => {
    if (!ownsSelection(store, pending)) {
      pending.cancel();
      return;
    }
    const current = store.getState().taskSessions.items[pending.previousSessionId];
    const changed =
      current?.state !== observed?.state || current?.updated_at !== observed?.updated_at;
    observed = current;
    if (pending.source === "primary" && changed) {
      pending.authorized = false;
      if (allowsFollowState(pending.source, current?.state) && !recheckQueued) {
        controller.abort();
        recheckQueued = true;
        queueMicrotask(() => {
          recheckQueued = false;
          if (!ownsPendingFollow(store, pending)) return;
          if (
            !allowsFollowState(
              pending.source,
              store.getState().taskSessions.items[pending.previousSessionId]?.state,
            )
          )
            return;
          pending.cancel();
          verifyPreviousSession(store, pending);
        });
      }
      return;
    }
    reconcileAuthorizedFollow(store, pending);
  });
  const releaseRestart = getSessionRestartStore(store).subscribe(() => {
    if (isSessionRestartPending(store, pending.previousSessionId)) pending.cancel();
  });
  const finish = () => {
    releaseView();
    releaseRestart();
    if (pendingByStore.get(store) === pending) pendingByStore.delete(store);
  };
  pendingByStore.set(store, pending);
  const read =
    pending.source === "removed"
      ? verifyRemoval(store, pending)
      : fetchTaskSession(pending.previousSessionId, {
          cache: "no-store",
          init: { signal: controller.signal },
        })
          .then(({ session }) => {
            if (controller.signal.aborted) return;
            const cached = store.getState().taskSessions.items[pending.previousSessionId]?.state;
            if (
              session.id !== pending.previousSessionId ||
              session.task_id !== pending.taskId ||
              !allowsFollowState(pending.source, session.state) ||
              (cached && !allowsFollowState(pending.source, cached))
            )
              return;
            pending.lifecycleRecipient ||= allowsFollowState("terminal", session.state);
            pending.authorized = true;
            reconcileAuthorizedFollow(store, pending);
          })
          .catch((error: unknown) => {
            if (!(error instanceof ApiError) || error.status !== 404) throw error;
            pending.source = "removed";
            return verifyRemoval(store, pending);
          });
  void read.catch(() => {
    if (!controller.signal.aborted) {
      // i18n-exempt: Internal diagnostic, never rendered as product copy.
      debug("primary follow verification failed", {
        task_id: pending.taskId,
        session_id: pending.previousSessionId,
      });
    }
  });
}

function captureOwner(
  state: AppState,
  taskId: string,
  previousSessionId: string,
  targetSessionId: string | null,
  source: FollowSource,
): FollowOwner {
  return {
    taskId,
    previousSessionId,
    selectedSessionId: previousSessionId,
    targetSessionId,
    source,
    lifecycleRecipient: source === "terminal" || source === NEW_SESSION_SOURCE,
    scopeIdentity: stateReadScopeIdentity(state),
    projectedPrimaryId:
      source === "primary" ? targetSessionId : findTaskInState(state, taskId)?.primarySessionId,
    navigationRevision: state.taskRemoval?.navigationRevision,
    pinnedSessionId: state.tasks.pinnedSessionId,
    workflowFocus: state.workflowSessionFocus,
  };
}

function reverifyLifecycleFollow(
  store: StoreApi<AppState>,
  pending: PendingFollow,
  source: "terminal" | "new-session",
): void {
  if (pending.source === "removed") return;
  pending.lifecycleRecipient ||= source === "terminal";
  pending.cancel();
  verifyPreviousSession(store, pending);
}

/** Only initial adoption without a same-task predecessor may bypass verification. */
export function followSessionTransition(
  store: StoreApi<AppState>,
  taskId: string,
  targetSessionId: string | undefined,
  source: "terminal" | "new-session",
): boolean {
  const state = store.getState();
  if (state.tasks.activeTaskId !== taskId) return true;
  if (hasExplicitWorkflowFocus(state, taskId)) return true;
  if (getProvenRemovalPredecessor(store, taskId, null)) return true;
  const existing = pendingByStore.get(store);
  if (existing?.taskId === taskId && ownsSelection(store, existing)) {
    reverifyLifecycleFollow(store, existing, source);
    return true;
  }
  const previous = state.tasks.activeSessionId;
  const unavailable = isUnavailableSession(state, targetSessionId, observedRemovals(store));
  if (!hasSameTaskPredecessor(state, taskId, previous)) return unavailable;
  if (source === NEW_SESSION_SOURCE && (!targetSessionId || targetSessionId === previous))
    return true;
  existing?.cancel();
  const owner = captureOwner(
    state,
    taskId,
    previous,
    unavailable ? null : (targetSessionId ?? null),
    source,
  );
  if (ownsSelection(store, owner)) verifyPreviousSession(store, owner);
  return true;
}

function captureRemovalOwner(
  store: StoreApi<AppState>,
  sessionId: string,
  taskId?: string,
): FollowOwner | undefined {
  const state = store.getState();
  const knownTask =
    state.taskSessions.items[sessionId]?.task_id ??
    (taskId && state.taskSessionsByTask.itemsByTaskId[taskId]?.some((row) => row.id === sessionId)
      ? taskId
      : undefined);
  const owningTask =
    knownTask ?? (state.tasks.activeSessionId === sessionId ? state.tasks.activeTaskId : null);
  if (!owningTask || (taskId && taskId !== owningTask)) return;
  return captureOwner(state, owningTask, sessionId, null, "removed");
}

/** Capture view ownership before admitting a local deletion request. */
export function prepareSessionRemoval(
  store: StoreApi<AppState>,
  sessionId: string,
  taskId: string,
): () => void {
  const captured = captureRemovalOwner(store, sessionId, taskId);
  return () => {
    if (!captured || stateReadScopeIdentity(store.getState()) !== captured.scopeIdentity) return;
    if (isConfirmedSessionRemoved(store.getState().taskSessions, sessionId)) return;
    const owner =
      store.getState().tasks.activeSessionId === sessionId
        ? (captureRemovalOwner(store, sessionId, taskId) ?? captured)
        : captured;
    observeRemoval(store, owner.taskId, sessionId);
    const existing = pendingByStore.get(store);
    if (existing?.taskId === owner.taskId && existing.previousSessionId === sessionId)
      existing.cancel();
    const owned = ownsSelection(store, owner);
    if (owned) {
      owner.selectedSessionId = null;
      owner.pinnedSessionId =
        store.getState().tasks.pinnedSessionId === sessionId
          ? null
          : store.getState().tasks.pinnedSessionId;
      owner.projectedPrimaryId = findTaskInState(store.getState(), owner.taskId)?.primarySessionId;
    }
    retainEmptySelection(store, owner);
    retireRemovalMembership(store, owner);
    if (owned && ownsSelection(store, owner)) verifyPreviousSession(store, owner);
  };
}

/** Committed removal uses known membership or matching active identity for task scope. */
export function followRemovedSession(
  store: StoreApi<AppState>,
  sessionId: string | undefined,
  taskId?: string,
): void {
  if (!sessionId) return;
  const owner = captureRemovalOwner(store, sessionId, taskId);
  if (!owner) return;
  const owningTask = owner.taskId;
  observeRemoval(store, owningTask, sessionId);
  if (ownsSelection(store, owner)) {
    pendingByStore.get(store)?.cancel();
    verifyPreviousSession(store, owner);
  } else {
    void verifyRemoval(store, owner).catch(() => {
      // i18n-exempt: Internal diagnostic, never rendered as product copy.
      debug("session removal verification failed", { task_id: owningTask, session_id: sessionId });
    });
  }
}

function reusePendingProjection(
  store: StoreApi<AppState>,
  pending: PendingFollow | undefined,
  taskId: string,
  targetSessionId: string,
): boolean {
  if (pending?.taskId !== taskId) return false;
  if (pending.projectedPrimaryId === targetSessionId) return true;
  if (pending.source === "removed") invalidateTaskSessionMembership(store, taskId);
  return false;
}

function retargetFollow(
  store: StoreApi<AppState>,
  existing: PendingFollow | undefined,
  taskId: string,
  previousSessionId: string,
  targetSessionId: string,
): void {
  const empty = getProvenRemovalPredecessor(store, taskId, null);
  const source =
    empty || (existing?.taskId === taskId && existing.source === "removed") ? "removed" : "primary";
  const owner = captureOwner(store.getState(), taskId, previousSessionId, targetSessionId, source);
  if (empty) owner.selectedSessionId = null;
  if (existing && ownsSelection(store, existing)) {
    owner.selectedSessionId = existing.selectedSessionId;
    owner.lifecycleRecipient = existing.lifecycleRecipient;
  }
  existing?.cancel();
  if (ownsSelection(store, owner)) verifyPreviousSession(store, owner);
}

function isPassiveProjection(
  pending: PendingFollow | undefined,
  taskId: string,
  retargetOnly: boolean,
  empty: boolean,
  unchanged: boolean,
): boolean {
  return (retargetOnly && pending?.taskId !== taskId) || (!pending && empty && unchanged);
}

/** Primary projection and visible conversation are separate ownership decisions. */
export function followUpdatedTaskPrimary(
  store: StoreApi<AppState>,
  taskId: string,
  previousPrimary: string | null,
  retargetOnly = false,
): void {
  const state = store.getState();
  if (state.tasks.activeTaskId !== taskId) return;
  const targetSessionId = findTaskInState(state, taskId)?.primarySessionId;
  const existing = pendingByStore.get(store);
  const removed = getProvenRemovalPredecessor(store, taskId, null);
  if (
    isPassiveProjection(
      existing,
      taskId,
      retargetOnly,
      !!removed,
      targetSessionId === previousPrimary,
    )
  )
    return;
  const previousSessionId =
    removed ?? (existing?.taskId === taskId ? existing.previousSessionId : previousPrimary);
  if (!targetSessionId || !previousSessionId || targetSessionId === previousSessionId) {
    if (existing?.source === "primary") existing.cancel();
    return;
  }
  if (reusePendingProjection(store, existing, taskId, targetSessionId)) return;
  retargetFollow(store, existing, taskId, previousSessionId, targetSessionId);
}
