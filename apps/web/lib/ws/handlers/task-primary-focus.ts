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
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import { createDebugLogger } from "@/lib/debug/log";
import {
  getSessionRestartStore,
  isSessionRestartPending,
} from "@/lib/session/session-restart-state";
import {
  clearPinnedSessionIfOverridden,
  isTerminalSessionState,
  pickReplacementSessionId,
  shouldPreservePinnedSessionForTask,
} from "./session-selection";
import type { KanbanTask } from "./task-archive-cache";

const debug = createDebugLogger("task-lifecycle:ws");
type FollowSource = "primary" | "terminal" | "new-session" | "removed";
type FollowOwner = {
  taskId: string;
  previousSessionId: string;
  targetSessionId: string | null;
  projectedPrimaryId: string | null | undefined;
  source: FollowSource;
  scopeIdentity: string;
  navigationRevision: number | undefined;
  pinnedSessionId: string | null;
  workflowFocus: AppState["workflowSessionFocus"];
};
type PendingFollow = FollowOwner & { cancel: () => void };
type RemovalOwner = Pick<FollowOwner, "taskId" | "previousSessionId" | "scopeIdentity">;
type RemovalTarget = Pick<TaskSession, "id" | "task_id" | "state" | "started_at" | "updated_at">;
// Denied or uncertain reads retain view ownership until selection changes.
const pendingByStore = new WeakMap<StoreApi<AppState>, PendingFollow>();

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
    state.tasks.activeSessionId === pending.previousSessionId &&
    state.tasks.pinnedSessionId === pending.pinnedSessionId &&
    state.taskRemoval?.navigationRevision === pending.navigationRevision &&
    state.workflowSessionFocus === pending.workflowFocus &&
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

function prepareFollowTarget(
  state: AppState,
  pending: FollowOwner,
  removedTarget?: RemovalTarget,
): string | null {
  let target = removedTarget?.id ?? pending.targetSessionId;
  if (pending.source === "terminal" || pending.source === "new-session") {
    const candidate = target ? state.taskSessions.items[target] : undefined;
    if (
      !candidate?.state ||
      candidate.task_id !== pending.taskId ||
      isTerminalSessionState(candidate.state)
    ) {
      target = pickReplacementSessionId(state, pending.taskId);
    }
  }
  if (
    !target ||
    target === pending.previousSessionId ||
    isConfirmedSessionRemoved(state.taskSessions, target)
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

function inheritFallbackReadiness(state: AppState, predecessor: string, target: string): void {
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

function completePendingFollow(
  store: StoreApi<AppState>,
  pending: PendingFollow,
  removedTarget?: RemovalTarget,
): boolean {
  if (!ownsPendingFollow(store, pending)) return false;
  const state = store.getState();
  if (findTaskInState(state, pending.taskId)?.primarySessionId !== pending.projectedPrimaryId)
    return false;
  const target = prepareFollowTarget(state, pending, removedTarget);
  if (!target) return false;
  pending.cancel();
  if (!removedTarget && (pending.source === "terminal" || pending.source === "new-session")) {
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

function allowsFollowState(source: FollowSource, state: TaskSession["state"]): boolean {
  if (!state || state === "FAILED") return false;
  return (
    source === "primary" ||
    state === "COMPLETED" ||
    state === "CANCELLED" ||
    (source === "new-session" && state === "IDLE")
  );
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
    const state = store.getState();
    if (
      state.tasks.activeTaskId === pending.taskId &&
      state.tasks.activeSessionId === pending.previousSessionId
    ) {
      matchingView?.cancel();
      state.clearActiveSession();
    }
  }
  invalidateTaskSessionMembership(store, pending.taskId);
  retireSessionReads(store, pending.previousSessionId);
  store
    .getState()
    .removeTaskSession(pending.taskId, pending.previousSessionId, { confirmed: true });
}

async function verifyRemoval(store: StoreApi<AppState>, pending: RemovalOwner) {
  if (stateReadScopeIdentity(store.getState()) !== pending.scopeIdentity) return;
  const reads = getTaskSessionReads(store);
  const release = reads.retain(pending.taskId);
  try {
    while (
      reads.isCurrent(pending.taskId) &&
      !isConfirmedSessionRemoved(store.getState().taskSessions, pending.previousSessionId)
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

function verifyPreviousSession(store: StoreApi<AppState>, owner: FollowOwner): void {
  const controller = new AbortController();
  const pending: PendingFollow = {
    ...owner,
    cancel: () => {
      controller.abort();
      finish();
    },
  };
  const releaseView = store.subscribe(() => {
    if (!ownsSelection(store, pending)) pending.cancel();
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
            const cached = store.getState().taskSessions.items[pending.previousSessionId]?.state;
            if (
              session.id !== pending.previousSessionId ||
              session.task_id !== pending.taskId ||
              !allowsFollowState(pending.source, session.state) ||
              (cached && !allowsFollowState(pending.source, cached))
            )
              return;
            completePendingFollow(store, pending);
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
    targetSessionId,
    source,
    scopeIdentity: stateReadScopeIdentity(state),
    projectedPrimaryId:
      source === "primary" ? targetSessionId : findTaskInState(state, taskId)?.primarySessionId,
    navigationRevision: state.taskRemoval?.navigationRevision,
    pinnedSessionId: state.tasks.pinnedSessionId,
    workflowFocus: state.workflowSessionFocus,
  };
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
  const existing = pendingByStore.get(store);
  if (existing?.taskId === taskId && ownsSelection(store, existing)) {
    if (existing.source === "removed") return true;
    existing.cancel();
    verifyPreviousSession(store, existing);
    return true;
  }
  const previous = state.tasks.activeSessionId;
  const previousRow = previous ? state.taskSessions.items[previous] : undefined;
  if (!previous || (previousRow && previousRow.task_id !== taskId)) return false;
  if (!targetSessionId || targetSessionId === previous) return true;
  existing?.cancel();
  const owner = captureOwner(state, taskId, previous, targetSessionId, source);
  if (ownsSelection(store, owner)) verifyPreviousSession(store, owner);
  return true;
}

/** Committed removal uses known membership or matching active identity for task scope. */
export function followRemovedSession(
  store: StoreApi<AppState>,
  sessionId: string | undefined,
  taskId?: string,
): void {
  if (!sessionId) return;
  const state = store.getState();
  const knownTask =
    state.taskSessions.items[sessionId]?.task_id ??
    (taskId && state.taskSessionsByTask.itemsByTaskId[taskId]?.some((row) => row.id === sessionId)
      ? taskId
      : undefined);
  const owningTask =
    knownTask ?? (state.tasks.activeSessionId === sessionId ? state.tasks.activeTaskId : null);
  if (!owningTask || (taskId && taskId !== owningTask)) return;
  const owner = captureOwner(state, owningTask, sessionId, null, "removed");
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

/** Primary projection and visible conversation are separate ownership decisions. */
export function followUpdatedTaskPrimary(
  store: StoreApi<AppState>,
  taskId: string,
  previousPrimary: string | null,
): void {
  const state = store.getState();
  if (state.tasks.activeTaskId !== taskId) return;
  const targetSessionId = findTaskInState(state, taskId)?.primarySessionId;
  const existing = pendingByStore.get(store);
  const previousSessionId =
    existing?.taskId === taskId ? existing.previousSessionId : previousPrimary;
  if (!targetSessionId || !previousSessionId || targetSessionId === previousSessionId) {
    if (existing?.source === "primary") existing.cancel();
    return;
  }
  if (reusePendingProjection(store, existing, taskId, targetSessionId)) return;
  existing?.cancel();
  const source = existing?.source === "removed" ? "removed" : "primary";
  const pending = captureOwner(state, taskId, previousSessionId, targetSessionId, source);
  if (ownsSelection(store, pending)) verifyPreviousSession(store, pending);
}
