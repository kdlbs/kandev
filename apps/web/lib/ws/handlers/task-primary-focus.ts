import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { fetchTaskSession, listTaskSessions } from "@/lib/api/domains/session-api";
import { ApiError } from "@/lib/api/client";
import type { TaskSessionsResponse } from "@/lib/types/http";
import { retireSessionReads } from "@/lib/state/session-read-coordinator";
import {
  getTaskSessionReads,
  invalidateTaskSessionMembership,
  registerTaskSessionMembershipProof,
  forgetTaskSessionMembershipProof,
} from "@/lib/state/task-session-reads";
import {
  disprovesSessionRemoval,
  isConfirmedSessionRemoved,
  proveRemovedSessionMembership,
} from "@/lib/state/session-removal";
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
  resolveLifecycleTarget,
  allowsFollowState,
  NEW_SESSION_SOURCE,
  inheritFallbackReadiness,
} from "./session-selection";
import { findTaskInState } from "./task-lookup";
import {
  captureOwner,
  inheritFollowIntent,
  ownsSelection,
  type FollowOwner,
  type PendingFollow,
  type RemovalOwner,
  type RemovalTarget,
} from "./task-primary-focus-owner";

const debug = createDebugLogger("task-lifecycle:ws");
// Denied or uncertain reads retain view ownership until selection changes.
const pendingByStore = new WeakMap<StoreApi<AppState>, PendingFollow>();

function observeRemoval(store: StoreApi<AppState>, taskId: string, sessionId: string): void {
  if (recordObservedSessionRemoval(store, sessionId))
    invalidateTaskSessionMembership(store, taskId);
  const owner = {
    taskId,
    previousSessionId: sessionId,
    scopeIdentity: stateReadScopeIdentity(store.getState()),
  };
  retainRemovalMembershipProof(store, owner);
}

function retainRemovalMembershipProof(store: StoreApi<AppState>, owner: RemovalOwner): void {
  registerTaskSessionMembershipProof(store, owner.taskId, owner.previousSessionId, (membership) => {
    commitRemovalMembership(store, owner, membership);
  });
}

function ownsPendingFollow(store: StoreApi<AppState>, pending: PendingFollow): boolean {
  return pendingByStore.get(store) === pending && ownsSelection(store, pending);
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
      ? resolveLifecycleTarget(state, pending, excluded)
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
  if (!ownsEmptySelection(store, pending))
    forgetTaskSessionMembershipProof(store, pending.previousSessionId);
  if (isConfirmedSessionRemoved(store.getState().taskSessions, pending.previousSessionId)) return;
  invalidateTaskSessionMembership(store, pending.taskId);
  retireSessionReads(store, pending.previousSessionId);
  store
    .getState()
    .removeTaskSession(pending.taskId, pending.previousSessionId, { confirmed: true });
  observedRemovals(store)?.delete(pending.previousSessionId);
}

/** A disproved removal releases its observation and any removal-sourced view owner. */
function withdrawRemoval(store: StoreApi<AppState>, pending: RemovalOwner): void {
  observedRemovals(store)?.delete(pending.previousSessionId);
  forgetTaskSessionMembershipProof(store, pending.previousSessionId);
  const view = pendingByStore.get(store);
  if (view?.taskId !== pending.taskId || view.selectedSessionId === null) return;
  if (view.source !== "removed" || view.previousSessionId !== pending.previousSessionId) return;
  view.cancel();
  // Intent that arrived while removal was unproven resumes as ordinary verification.
  if (!view.deferred) return;
  const { taskId, previousSessionId, selectedSessionId, lifecycleRecipient } = view;
  const { source, targetSessionId } = view.deferred;
  const owner = captureOwner(store.getState(), taskId, previousSessionId, targetSessionId, source);
  Object.assign(owner, { selectedSessionId, resumedAfterDisproof: true });
  owner.lifecycleRecipient ||= lifecycleRecipient;
  if (ownsSelection(store, owner)) verifyPreviousSession(store, owner);
}

function commitRemovalMembership(
  store: StoreApi<AppState>,
  pending: RemovalOwner,
  membership: TaskSessionsResponse,
) {
  const proof = proveRemovedSessionMembership(
    membership,
    pending.taskId,
    pending.previousSessionId,
  );
  if (!proof) {
    if (disprovesSessionRemoval(membership, pending.taskId, pending.previousSessionId))
      withdrawRemoval(store, pending);
    return;
  }
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

const MAX_REMOVAL_VERIFY_ATTEMPTS = 5;

async function verifyRemoval(store: StoreApi<AppState>, pending: RemovalOwner) {
  if (stateReadScopeIdentity(store.getState()) !== pending.scopeIdentity) return;
  const reads = getTaskSessionReads(store);
  const release = reads.retain(pending.taskId);
  let attempts = 0;
  try {
    while (
      attempts++ < MAX_REMOVAL_VERIFY_ATTEMPTS &&
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
    if (pendingByStore.get(store) === pending) {
      pendingByStore.delete(store);
      if (isConfirmedSessionRemoved(store.getState().taskSessions, pending.previousSessionId))
        forgetTaskSessionMembershipProof(store, pending.previousSessionId);
    }
  };
  pendingByStore.set(store, pending);
  if (pending.source === "removed") retainRemovalMembershipProof(store, pending);
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
            if (
              controller.signal.aborted ||
              stateReadScopeIdentity(store.getState()) !== pending.scopeIdentity
            )
              return;
            return convertToRemoval(store, pending);
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

/** A 404 converts the owner to removal verification, deferring its original intent once. */
function convertToRemoval(store: StoreApi<AppState>, pending: PendingFollow): Promise<void> {
  observeRemoval(store, pending.taskId, pending.previousSessionId);
  const { source, targetSessionId } = pending;
  if (!pending.resumedAfterDisproof) pending.deferred ??= { source, targetSessionId };
  pending.source = "removed";
  return verifyRemoval(store, pending);
}

function reverifyLifecycleFollow(
  store: StoreApi<AppState>,
  pending: PendingFollow,
  source: "terminal" | "new-session",
  targetSessionId: string | undefined,
): void {
  pending.lifecycleRecipient ||= source === "terminal";
  if (pending.source === "removed") {
    pending.deferred ??= { source, targetSessionId: targetSessionId ?? null };
    return;
  }
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
    reverifyLifecycleFollow(store, existing, source, targetSessionId);
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
    const existing = pendingByStore.get(store);
    inheritFollowIntent(owner, existing);
    existing?.cancel();
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
  if (source === "removed") owner.deferred = { source: "primary", targetSessionId };
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
