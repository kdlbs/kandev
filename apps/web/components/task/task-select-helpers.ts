/**
 * Helpers for task selection in the sidebar. Extracted as pure functions so
 * the no-session fallback path can be unit-tested without standing up the
 * dockview runtime.
 */

import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { TaskPendingAction, TaskSession } from "@/lib/types/http";
import { performLayoutSwitch, releaseLayoutToDefault } from "@/lib/state/dockview-store";
import { replaceTaskUrl } from "@/lib/links";
import { launchSession } from "@/lib/services/session-launch-service";
import { buildPrepareRequest } from "@/lib/services/session-launch-helpers";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import { isInputCapableSessionState } from "@/lib/utils/task-pending-input";
import { isAbortError } from "@/lib/utils/abort-error";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";

const debug = createDebugLogger("dockview:task-select");
let taskSelectionSequence = 0;

export type SwitchToSessionFn = (
  taskId: string,
  sessionId: string,
  oldSessionId: string | null | undefined,
) => void;

type TaskSessionLoader = (
  taskId: string,
  options?: { force?: boolean; signal?: AbortSignal },
) => Promise<TaskSession[]>;

type PendingTask = {
  taskPendingAction?: TaskPendingAction | null;
  statusSummary?: {
    revision?: number;
    updated_at?: string;
    pending_action?: TaskPendingAction | null;
  } | null;
};

export type TaskPendingSelectionSnapshot = {
  revision: number | null;
  pendingAction: TaskPendingAction | null | undefined;
};

export function taskPendingSelectionMatches(
  initial: TaskPendingSelectionSnapshot,
  current: TaskPendingSelectionSnapshot,
): boolean {
  return current.revision === initial.revision && current.pendingAction === initial.pendingAction;
}

export function effectiveTaskPendingAction(
  task: PendingTask | undefined,
): TaskPendingAction | null | undefined {
  return task?.statusSummary != null ? task.statusSummary.pending_action : task?.taskPendingAction;
}

export function taskPendingSelectionSnapshot(
  task: PendingTask | undefined,
): TaskPendingSelectionSnapshot {
  return {
    revision: task?.statusSummary?.revision ?? null,
    pendingAction: effectiveTaskPendingAction(task),
  };
}

function handlePendingSelectionOwnerChange(
  store: StoreApi<AppState>,
  taskId: string,
  initial: TaskPendingSelectionSnapshot,
  taskProjectionExisted: boolean,
  onChanged: () => void,
): boolean {
  if (!initial.pendingAction) return false;
  const state = store.getState();
  const currentTask = findTaskInSnapshots(
    taskId,
    state.kanbanMulti?.snapshots ?? {},
    state.kanban?.tasks ?? [],
  );
  if (!currentTask) {
    // Callers without an initial store projection cannot revalidate existence.
    // A projected task must fail closed if it disappears during session loading.
    return taskProjectionExisted;
  }
  const current = taskPendingSelectionSnapshot(currentTask);
  if (taskPendingSelectionMatches(initial, current)) return false;
  onChanged();
  return true;
}

function pendingOwnerGuard(
  store: StoreApi<AppState>,
  taskId: string,
  task: PendingTask | undefined,
  onChanged: () => void,
): () => boolean {
  const state = store.getState();
  const initial = taskPendingSelectionSnapshot(task);
  const taskProjectionExisted = Boolean(
    findTaskInSnapshots(taskId, state.kanbanMulti?.snapshots ?? {}, state.kanban?.tasks ?? []),
  );
  return () =>
    handlePendingSelectionOwnerChange(store, taskId, initial, taskProjectionExisted, onChanged);
}

function getTaskSessionIds(state: AppState, taskId: string): string[] {
  return (state.taskSessionsByTask?.itemsByTaskId?.[taskId] ?? []).map((session) => session.id);
}

export function resolveLoadedSessionId(
  sessions: TaskSession[],
  preferredSessionId: string,
): string {
  return (
    sessions.find((s) => s.id === preferredSessionId)?.id ??
    sessions.find((s) => s.is_primary)?.id ??
    sessions[0]?.id ??
    preferredSessionId
  );
}

export function resolveTaskSessionId(args: {
  sessions: TaskSession[];
  preferredSessionId: string;
  taskPendingAction?: TaskPendingAction | null;
}): string {
  const { sessions, preferredSessionId, taskPendingAction } = args;
  if (taskPendingAction) {
    const owner = sessions.find(
      (session) =>
        isInputCapableSessionState(session.state) && session.pending_action === taskPendingAction,
    );
    return owner?.id ?? "";
  }
  return resolveLoadedSessionId(sessions, preferredSessionId);
}

/**
 * Pick the session to re-open when the user navigates back to a task.
 *
 * Prefers the user's last-selected session (tracked per task in
 * `lastSessionByTaskId`) over `primarySessionId`, so opening a non-primary
 * tab then bouncing through another task does not silently snap the user
 * back to primary. Falls back to `primarySessionId` when the remembered
 * session is unknown / missing an env mapping (e.g. it was deleted), OR
 * when it belongs to a different task — the latter guards against a poisoned
 * `lastSessionByTaskId` entry written by a stale dockview panel-activation
 * during a task switch (see `setupSessionTabSync`).
 */
export function resolvePreferredSessionId(args: {
  taskId: string;
  primarySessionId: string;
  lastSessionByTaskId: Record<string, string>;
  environmentIdBySessionId: Record<string, string>;
  taskSessionsById: Record<string, TaskSession>;
}): string {
  const {
    taskId,
    primarySessionId,
    lastSessionByTaskId,
    environmentIdBySessionId,
    taskSessionsById,
  } = args;
  const last = lastSessionByTaskId[taskId];
  if (!last || !environmentIdBySessionId[last]) return primarySessionId;
  const lastTaskId = taskSessionsById[last]?.task_id;
  if (lastTaskId && lastTaskId !== taskId) return primarySessionId;
  return last;
}

export function buildSwitchToSession(
  store: StoreApi<AppState>,
  setActiveSession: (taskId: string, sessionId: string) => void,
): SwitchToSessionFn {
  return (taskId, sessionId, oldSessionId) => {
    const state = store.getState();
    const oldEnvId = oldSessionId ? (state.environmentIdBySessionId[oldSessionId] ?? null) : null;
    const newEnvId = state.environmentIdBySessionId[sessionId] ?? null;
    if (isDebug()) {
      debug("switchToSession: entry", {
        taskId,
        sessionId,
        oldSessionId: oldSessionId ?? null,
        oldEnvId,
        newEnvId,
        path: newEnvId ? "performLayoutSwitch" : "releaseToDefault",
      });
    }
    setActiveSession(taskId, sessionId);
    if (newEnvId) {
      performLayoutSwitch(oldEnvId, newEnvId, sessionId, getTaskSessionIds(state, taskId));
      return;
    }
    // The new session's task_environment_id has not been loaded into the store
    // yet (e.g. auto-started sessions whose WS payload hasn't arrived). If we
    // skip the layout switch entirely, env-scoped panels from the outgoing
    // task (plan, files, vscode, …) remain visible. Release the outgoing env's
    // layout to default so the new task starts from a clean slate; when the
    // new env id arrives, useEnvSwitchCleanup will adopt it without rebuild.
    if (oldEnvId || oldSessionId !== sessionId) {
      if (isDebug()) {
        debug("switchToSession: releasing outgoing env (no newEnvId yet)", { oldEnvId });
      }
      releaseLayoutToDefault(oldEnvId);
    }
  };
}

function nextTaskSelectionToken(): number {
  taskSelectionSequence += 1;
  return taskSelectionSequence;
}

function taskSelectionWasSuperseded(selectionToken: number): boolean {
  return selectionToken !== taskSelectionSequence;
}

function createTaskSelectionGuard(
  store: StoreApi<AppState>,
  taskId: string,
  selectionToken: number,
  selectionSignal?: AbortSignal,
) {
  const startActiveTaskId = store.getState().tasks.activeTaskId ?? null;
  let activeTaskChangedExternally = false;
  const unsubscribe = store.subscribe((current, previous) => {
    const currentTaskId = current.tasks.activeTaskId ?? null;
    const previousTaskId = previous.tasks.activeTaskId ?? null;
    if (currentTaskId !== previousTaskId && currentTaskId !== taskId) {
      activeTaskChangedExternally = true;
    }
  });
  return {
    dispose: () => {
      if (typeof unsubscribe === "function") unsubscribe();
    },
    wasSuperseded: () => {
      if (
        selectionSignal?.aborted ||
        taskSelectionWasSuperseded(selectionToken) ||
        activeTaskChangedExternally
      ) {
        return true;
      }
      const activeTaskId = store.getState().tasks.activeTaskId ?? null;
      return activeTaskId !== startActiveTaskId && activeTaskId !== taskId;
    },
  };
}

export async function prepareAndSwitchTask(
  taskId: string,
  store: StoreApi<AppState>,
  switchToSession: SwitchToSessionFn,
  setPreparingTaskId: (id: string | null) => void,
  shouldContinue: () => boolean = () => true,
): Promise<boolean> {
  setPreparingTaskId(taskId);
  // Capture before the async launch; WS events may update activeSessionId
  // before launchSession resolves, causing a layout switch with the wrong old session.
  const oldSessionId = store.getState().tasks.activeSessionId;
  // Release the outgoing env BEFORE awaiting `launchSession`. Otherwise the
  // old task's env-scoped panels (file-editor, diff-viewer, commit-detail,
  // browser, vscode, pr-detail) stay mounted in the dockview for the entire
  // round-trip + WS env-id propagation, leaking into the new (preparing)
  // task as stray tabs.
  const oldEnvId = oldSessionId
    ? (store.getState().environmentIdBySessionId[oldSessionId] ?? null)
    : null;
  releaseLayoutToDefault(oldEnvId);
  try {
    const { request } = buildPrepareRequest(taskId);
    const resp = await launchSession(request);
    if (!shouldContinue()) return false;
    if (resp.session_id) {
      // Pass `null` instead of the original oldSessionId — releaseLayoutToDefault
      // already saved + released the outgoing env, and the dockview now holds the
      // default layout. If we forwarded oldSessionId, the subsequent
      // switchEnvLayout would call saveOutgoingEnv(envA) a second time and
      // overwrite envA's correctly-persisted layout with the default.
      switchToSession(taskId, resp.session_id, null);
      return true;
    }
    return false;
  } catch {
    return false;
  } finally {
    setPreparingTaskId(null);
  }
}

type SelectTaskWithLayoutParams = {
  taskId: string;
  task:
    | {
        primarySessionId?: string | null;
        taskPendingAction?: TaskPendingAction | null;
        statusSummary?: {
          revision?: number;
          updated_at?: string;
          pending_action?: TaskPendingAction | null;
        } | null;
        isArchived?: boolean;
      }
    | undefined;
  store: StoreApi<AppState>;
  switchToSession: SwitchToSessionFn;
  loadTaskSessionsForTask: TaskSessionLoader;
  setActiveTask: (taskId: string) => void;
  setPreparingTaskId: (id: string | null) => void;
  navigateToTask?: (taskId: string) => void;
  selectionSignal?: AbortSignal;
};

function loadTaskSessionsForSelection(
  params: SelectTaskWithLayoutParams,
  hasPendingAction: boolean,
): Promise<TaskSession[]> {
  if (!hasPendingAction && !params.selectionSignal) {
    return params.loadTaskSessionsForTask(params.taskId);
  }
  const options = { force: hasPendingAction || undefined, signal: params.selectionSignal };
  return params.loadTaskSessionsForTask(params.taskId, options);
}

function openTaskWithoutSession(
  params: SelectTaskWithLayoutParams,
  navigateToTask: (taskId: string) => void,
): void {
  const state = params.store.getState();
  const oldSessionId = state.tasks.activeSessionId;
  const oldEnvId = oldSessionId ? (state.environmentIdBySessionId[oldSessionId] ?? null) : null;
  releaseLayoutToDefault(oldEnvId);
  params.setActiveTask(params.taskId);
  navigateToTask(params.taskId);
}

function logTaskSelection(
  taskId: string,
  primarySessionId: string | null | undefined,
  oldSessionId: string | null | undefined,
  previousTaskId: string | null | undefined,
): void {
  if (!isDebug()) return;
  debug("selectTaskWithLayout: entry", {
    taskId,
    primarySessionId: primarySessionId ?? null,
    oldSessionId: oldSessionId ?? null,
    prevActiveTaskId: previousTaskId ?? null,
  });
}

export function selectTaskWithLayout(params: SelectTaskWithLayoutParams): void {
  const { taskId, task, store, switchToSession, loadTaskSessionsForTask } = params;
  const state = store.getState();
  const oldSessionId = state.tasks.activeSessionId;
  const navigateToTask = params.navigateToTask ?? replaceTaskUrl;
  const openWithoutSession = () => openTaskWithoutSession(params, navigateToTask);
  const taskPendingAction = effectiveTaskPendingAction(task);
  const pendingOwnerHandled = pendingOwnerGuard(store, taskId, task, openWithoutSession);
  const selectionGuard = createTaskSelectionGuard(
    store,
    taskId,
    nextTaskSelectionToken(),
    params.selectionSignal,
  );
  logTaskSelection(taskId, task?.primarySessionId, oldSessionId, state.tasks.activeTaskId);
  if (task?.isArchived) {
    selectionGuard.dispose();
    params.setActiveTask(taskId);
    navigateToTask(taskId);
    return;
  }
  if (task?.primarySessionId) {
    const targetSessionId = resolvePreferredSessionId({
      taskId,
      primarySessionId: task.primarySessionId,
      lastSessionByTaskId: state.tasks.lastSessionByTaskId,
      environmentIdBySessionId: state.environmentIdBySessionId,
      taskSessionsById: state.taskSessions.items,
    });
    if (state.environmentIdBySessionId[targetSessionId] && !taskPendingAction) {
      selectionGuard.dispose();
      switchToSession(taskId, targetSessionId, oldSessionId);
      void loadTaskSessionsForTask(taskId).catch(() => undefined);
      navigateToTask(taskId);
      return;
    }
    void loadTaskSessionsForSelection(params, !!taskPendingAction)
      .then((sessions) => {
        if (selectionGuard.wasSuperseded()) return;
        if (pendingOwnerHandled()) return;
        const currentOldSessionId = store.getState().tasks.activeSessionId;
        const resolvedSessionId = resolveTaskSessionId({
          sessions,
          preferredSessionId: targetSessionId,
          taskPendingAction,
        });
        if (!resolvedSessionId) return openWithoutSession();
        switchToSession(taskId, resolvedSessionId, currentOldSessionId);
        navigateToTask(taskId);
      })
      .catch((error) => {
        if (isAbortError(error)) return;
        if (selectionGuard.wasSuperseded()) return;
        if (pendingOwnerHandled()) return;
        if (taskPendingAction) return openWithoutSession();
        switchToSession(taskId, targetSessionId, store.getState().tasks.activeSessionId);
        navigateToTask(taskId);
      })
      .finally(selectionGuard.dispose);
    return;
  }
  void loadTaskSessionsForSelection(params, !!taskPendingAction)
    .then(async (sessions) => {
      if (selectionGuard.wasSuperseded()) return;
      if (pendingOwnerHandled()) return;
      const currentOldSessionId = store.getState().tasks.activeSessionId;
      const sessionId = resolveTaskSessionId({
        sessions,
        preferredSessionId: "",
        taskPendingAction,
      });
      if (sessionId) {
        switchToSession(taskId, sessionId, currentOldSessionId);
        navigateToTask(taskId);
        return;
      }
      if (taskPendingAction) return openWithoutSession();

      const switched = await prepareAndSwitchTask(
        taskId,
        store,
        switchToSession,
        params.setPreparingTaskId,
        () => !selectionGuard.wasSuperseded(),
      );
      if (switched) {
        navigateToTask(taskId);
        return;
      }
      if (selectionGuard.wasSuperseded()) return;

      // prepareAndSwitchTask already saved and released the outgoing layout.
      // Releasing again would overwrite it with the current default layout.
      params.setActiveTask(taskId);
      navigateToTask(taskId);
    })
    .catch((error) => {
      if (isAbortError(error)) return;
      if (selectionGuard.wasSuperseded()) return;
      if (pendingOwnerHandled()) return;
      openWithoutSession();
    })
    .finally(selectionGuard.dispose);
}
