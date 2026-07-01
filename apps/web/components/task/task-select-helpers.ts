/**
 * Helpers for task selection in the sidebar. Extracted as pure functions so
 * the no-session fallback path can be unit-tested without standing up the
 * dockview runtime.
 */

import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";
import {
  performLayoutSwitch,
  releaseLayoutToDefault,
  useDockviewStore,
} from "@/lib/state/dockview-store";
import { INTENT_PR_REVIEW } from "@/lib/state/layout-manager";
import { replaceTaskUrl } from "@/lib/links";
import { getBrowserQueryClient } from "@/lib/query/client";
import { qk } from "@/lib/query/keys";
import { fetchTaskSession } from "@/lib/api/domains/session-api";
import { launchSession } from "@/lib/services/session-launch-service";
import { buildPrepareRequest } from "@/lib/services/session-launch-helpers";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import { sessionHasRoutingInfo } from "@/lib/session/task-session-navigation";

const debug = createDebugLogger("dockview:task-select");
let taskSelectionSequence = 0;

export type SwitchToSessionFn = (
  taskId: string,
  sessionId: string,
  oldSessionId: string | null | undefined,
) => void;

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

async function hydrateSessionRoutingInfo(
  store: StoreApi<AppState>,
  sessionId: string,
  session: TaskSession | undefined,
): Promise<void> {
  if (sessionHasRoutingInfo(session)) return;
  try {
    const response = await fetchTaskSession(sessionId, { cache: "no-store" });
    if (!response?.session) return;
    store.getState().setTaskSession(response.session);
    getBrowserQueryClient().setQueryData(qk.taskSession.byId(sessionId), response.session);
  } catch (error) {
    console.warn("[task-select] failed to hydrate session routing info", error);
  }
}

async function resolveLoadedSessionIdForSwitch(
  store: StoreApi<AppState>,
  sessions: TaskSession[],
  preferredSessionId: string,
): Promise<string> {
  const sessionId = resolveLoadedSessionId(sessions, preferredSessionId);
  const session = sessions.find((item) => item.id === sessionId);
  await hydrateSessionRoutingInfo(store, sessionId, session);
  return sessionId;
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

function debugTaskSelectionEntry(args: {
  taskId: string;
  primarySessionId: string | null;
  oldSessionId: string | null | undefined;
  prevActiveTaskId: string | null;
}) {
  if (!isDebug()) return;
  debug("selectTaskWithLayout: entry", args);
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
      const taskPRs = getBrowserQueryClient().getQueryData<unknown>(
        qk.integrations.github.taskPr(taskId),
      );
      if (Array.isArray(taskPRs) && taskPRs.length > 0) {
        const { api, buildDefaultLayout } = useDockviewStore.getState();
        if (api) buildDefaultLayout(api, INTENT_PR_REVIEW);
      }
      return true;
    }
    return false;
  } catch {
    return false;
  } finally {
    setPreparingTaskId(null);
  }
}

export function selectTaskWithLayout(params: {
  taskId: string;
  task: { primarySessionId?: string | null } | undefined;
  store: StoreApi<AppState>;
  switchToSession: SwitchToSessionFn;
  loadTaskSessionsForTask: (taskId: string) => Promise<TaskSession[]>;
  setActiveTask: (taskId: string) => void;
  setPreparingTaskId: (id: string | null) => void;
}): void {
  const { taskId, task, store, switchToSession, loadTaskSessionsForTask } = params;
  const state = store.getState();
  const selectionToken = nextTaskSelectionToken();
  const startActiveTaskId = state.tasks.activeTaskId ?? null;
  const oldSessionId = state.tasks.activeSessionId;
  let activeTaskChangedExternally = false;
  const unsubscribeSelectionGuard = store.subscribe((current, previous) => {
    const currentTaskId = current.tasks.activeTaskId ?? null;
    const previousTaskId = previous.tasks.activeTaskId ?? null;
    if (currentTaskId !== previousTaskId && currentTaskId !== taskId) {
      activeTaskChangedExternally = true;
    }
  });
  const disposeSelectionGuard = () => unsubscribeSelectionGuard?.();
  const selectionWasSuperseded = () => {
    if (taskSelectionWasSuperseded(selectionToken)) return true;
    if (activeTaskChangedExternally) return true;
    const activeTaskId = store.getState().tasks.activeTaskId ?? null;
    return activeTaskId !== startActiveTaskId && activeTaskId !== taskId;
  };
  debugTaskSelectionEntry({
    taskId,
    primarySessionId: task?.primarySessionId ?? null,
    oldSessionId: oldSessionId ?? null,
    prevActiveTaskId: state.tasks.activeTaskId ?? null,
  });
  if (task?.primarySessionId) {
    const targetSessionId = resolvePreferredSessionId({
      taskId,
      primarySessionId: task.primarySessionId,
      lastSessionByTaskId: state.tasks.lastSessionByTaskId,
      environmentIdBySessionId: state.environmentIdBySessionId,
      taskSessionsById: state.taskSessions.items,
    });
    const hasEnvId = Boolean(state.environmentIdBySessionId[targetSessionId]);
    const hasRoutingInfo = sessionHasRoutingInfo(state.taskSessions.items[targetSessionId]);
    if (hasEnvId && hasRoutingInfo) {
      disposeSelectionGuard();
      switchToSession(taskId, targetSessionId, oldSessionId);
      loadTaskSessionsForTask(taskId);
      replaceTaskUrl(taskId);
      return;
    }
    void loadTaskSessionsForTask(taskId)
      .then(async (sessions) => {
        if (selectionWasSuperseded()) return;
        const sessionId = await resolveLoadedSessionIdForSwitch(store, sessions, targetSessionId);
        if (selectionWasSuperseded()) return;
        switchToSession(taskId, sessionId, oldSessionId);
        replaceTaskUrl(taskId);
      })
      .finally(disposeSelectionGuard)
      .catch(() => undefined);
    return;
  }

  void loadTaskSessionsForTask(taskId)
    .then(async (sessions) => {
      if (selectionWasSuperseded()) return;
      const currentOldSessionId = store.getState().tasks.activeSessionId;
      const primary = sessions.find((s) => s.is_primary);
      const sessionId = primary?.id ?? sessions[0]?.id ?? null;
      if (sessionId) {
        await hydrateSessionRoutingInfo(
          store,
          sessionId,
          sessions.find((item) => item.id === sessionId),
        );
        if (selectionWasSuperseded()) return;
        switchToSession(taskId, sessionId, currentOldSessionId);
        replaceTaskUrl(taskId);
        return;
      }

      const switched = await prepareAndSwitchTask(
        taskId,
        store,
        switchToSession,
        params.setPreparingTaskId,
        () => !selectionWasSuperseded(),
      );
      if (switched) {
        replaceTaskUrl(taskId);
        return;
      }
      if (selectionWasSuperseded()) return;

      // Failure path: prepareAndSwitchTask already called releaseLayoutToDefault
      // before awaiting, so the outgoing env's layout is already saved and the
      // dockview is showing the default layout. A second release here would
      // overwrite the just-saved env layout with `api.toJSON()` (the default),
      // losing the user's real layout for the originating task.
      params.setActiveTask(taskId);
      replaceTaskUrl(taskId);
    })
    .finally(disposeSelectionGuard)
    .catch(() => undefined);
}
