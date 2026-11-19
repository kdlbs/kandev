import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { fetchTaskSession, listTaskSessions } from "@/lib/api/domains/session-api";
import { ApiError } from "@/lib/api/client";
import type { TaskSessionsResponse } from "@/lib/types/http";
import { retireSessionReads } from "@/lib/state/session-read-coordinator";
import { createDebugLogger } from "@/lib/debug/log";
import {
  getSessionRestartStore,
  isSessionRestartPending,
} from "@/lib/session/session-restart-state";
import {
  clearPinnedSessionIfOverridden,
  shouldPreservePinnedSessionForTask,
} from "./session-selection";
import type { KanbanTask } from "./task-archive-cache";

const debug = createDebugLogger("task-lifecycle:ws");
type FollowOwner = {
  taskId: string;
  previousSessionId: string;
  targetSessionId: string;
  navigationRevision: number | undefined;
  pinnedSessionId: string | null;
  workflowFocus: AppState["workflowSessionFocus"];
};
type PendingFollow = FollowOwner & { cancel: () => void };
// A denied or uncertain read still owns the pending primary change until selection changes.
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
    state.tasks.activeTaskId === pending.taskId &&
    state.tasks.activeSessionId === pending.previousSessionId &&
    state.tasks.pinnedSessionId === pending.pinnedSessionId &&
    state.taskRemoval?.navigationRevision === pending.navigationRevision &&
    state.workflowSessionFocus === pending.workflowFocus &&
    !isSessionRestartPending(store, pending.previousSessionId) &&
    !shouldPreservePinnedSessionForTask(state, pending.taskId) &&
    !!findTaskInState(state, pending.taskId)
  );
}

function completePendingFollow(store: StoreApi<AppState>, pending: PendingFollow): boolean {
  if (pendingByStore.get(store) !== pending || !ownsSelection(store, pending)) return false;
  const primary = findTaskInState(store.getState(), pending.taskId)?.primarySessionId;
  if (primary !== pending.targetSessionId) return false;
  pending.cancel();
  clearPinnedSessionIfOverridden(store, pending.targetSessionId);
  store.getState().setActiveSessionAuto(pending.taskId, pending.targetSessionId);
  return true;
}

function confirmsRemovedPredecessor(
  membership: TaskSessionsResponse,
  pending: FollowOwner,
): boolean {
  if (membership.total !== membership.sessions.length) return false;
  let targets = 0;
  for (const session of membership.sessions) {
    if (session.task_id !== pending.taskId || session.id === pending.previousSessionId)
      return false;
    if (session.id === pending.targetSessionId) targets += 1;
  }
  return targets === 1;
}

/** Lifecycle observations cannot bypass an owned authoritative primary check. */
export function deferOwnedPrimaryFollow(store: StoreApi<AppState>, taskId: string): boolean {
  const pending = pendingByStore.get(store);
  if (!pending || pending.taskId !== taskId) return false;
  if (!ownsSelection(store, pending)) return false;
  pending.cancel();
  verifyPreviousSession(store, pending);
  return true;
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
  void fetchTaskSession(pending.previousSessionId, {
    cache: "no-store",
    init: { signal: controller.signal },
  })
    .then(({ session }) => {
      if (
        session.id !== pending.previousSessionId ||
        session.task_id !== pending.taskId ||
        !session.state ||
        session.state === "FAILED" ||
        store.getState().taskSessions.items[pending.previousSessionId]?.state === "FAILED"
      )
        return;
      completePendingFollow(store, pending);
    })
    .catch(async (error: unknown) => {
      if (!(error instanceof ApiError) || error.status !== 404) throw error;
      if (pendingByStore.get(store) !== pending || !ownsSelection(store, pending)) return;
      const membership = await listTaskSessions(pending.taskId, {
        cache: "no-store",
        init: { signal: controller.signal },
      });
      if (
        confirmsRemovedPredecessor(membership, pending) &&
        completePendingFollow(store, pending)
      ) {
        retireSessionReads(store, pending.previousSessionId);
        store.getState().removeTaskSession(pending.taskId, pending.previousSessionId);
      }
    })
    .catch(() => {
      if (!controller.signal.aborted) {
        // i18n-exempt: Internal diagnostic, never rendered as product copy.
        debug("primary follow verification failed", {
          task_id: pending.taskId,
          session_id: pending.previousSessionId,
        });
      }
    });
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
    existing?.cancel();
    return;
  }
  if (existing?.taskId === taskId && existing.targetSessionId === targetSessionId) return;
  existing?.cancel();
  const pending: FollowOwner = {
    taskId,
    previousSessionId,
    targetSessionId,
    navigationRevision: state.taskRemoval?.navigationRevision,
    pinnedSessionId: state.tasks.pinnedSessionId,
    workflowFocus: state.workflowSessionFocus,
  };
  if (ownsSelection(store, pending)) verifyPreviousSession(store, pending);
}
