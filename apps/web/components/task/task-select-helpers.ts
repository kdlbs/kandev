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
import { launchSession } from "@/lib/services/session-launch-service";
import { buildPrepareRequest } from "@/lib/services/session-launch-helpers";

export type SwitchToSessionFn = (
  taskId: string,
  sessionId: string,
  oldSessionId: string | null | undefined,
) => void;

export type FinalizeNoSessionSelectDeps = {
  /** Set the new active task in the kanban store (also clears activeSessionId). */
  setActiveTask: (taskId: string) => void;
  /** Save the outgoing env's layout, release its portals, then build the default layout. */
  releaseLayoutToDefault: (oldEnvId: string | null) => void;
  /** Push the new task id into the URL without reloading. */
  replaceTaskUrl: (taskId: string) => void;
};

/**
 * Finalize a sidebar task selection when no session could be resolved or
 * launched for the new task. Releasing the dockview to default first ensures
 * portal cleanup targets the still-active outgoing env before
 * `setActiveTask` clears `activeSessionId` to null.
 */
export function finalizeNoSessionSelect(
  taskId: string,
  oldEnvId: string | null,
  deps: FinalizeNoSessionSelectDeps,
): void {
  deps.releaseLayoutToDefault(oldEnvId);
  deps.setActiveTask(taskId);
  deps.replaceTaskUrl(taskId);
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

export function buildSwitchToSession(
  store: StoreApi<AppState>,
  setActiveSession: (taskId: string, sessionId: string) => void,
): SwitchToSessionFn {
  return (taskId, sessionId, oldSessionId) => {
    const state = store.getState();
    const oldEnvId = oldSessionId ? (state.environmentIdBySessionId[oldSessionId] ?? null) : null;
    const newEnvId = state.environmentIdBySessionId[sessionId] ?? null;
    setActiveSession(taskId, sessionId);
    if (newEnvId) performLayoutSwitch(oldEnvId, newEnvId, sessionId);
  };
}

export async function prepareAndSwitchTask(
  taskId: string,
  store: StoreApi<AppState>,
  switchToSession: SwitchToSessionFn,
  setPreparingTaskId: (id: string | null) => void,
): Promise<boolean> {
  setPreparingTaskId(taskId);
  // Capture before the async launch; WS events may update activeSessionId
  // before launchSession resolves, causing a layout switch with the wrong old session.
  const oldSessionId = store.getState().tasks.activeSessionId;
  try {
    const { request } = buildPrepareRequest(taskId);
    const resp = await launchSession(request);
    if (resp.session_id) {
      switchToSession(taskId, resp.session_id, oldSessionId);
      if ((store.getState().taskPRs.byTaskId[taskId]?.length ?? 0) > 0) {
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
  const oldSessionId = store.getState().tasks.activeSessionId;
  if (task?.primarySessionId) {
    const primarySessionId = task.primarySessionId;
    const hasEnvId = !!store.getState().environmentIdBySessionId[primarySessionId];
    if (hasEnvId) {
      switchToSession(taskId, primarySessionId, oldSessionId);
      loadTaskSessionsForTask(taskId);
      replaceTaskUrl(taskId);
      return;
    }
    loadTaskSessionsForTask(taskId).then((sessions) => {
      switchToSession(taskId, resolveLoadedSessionId(sessions, primarySessionId), oldSessionId);
      replaceTaskUrl(taskId);
    });
    return;
  }

  loadTaskSessionsForTask(taskId).then(async (sessions) => {
    const currentOldSessionId = store.getState().tasks.activeSessionId;
    const primary = sessions.find((s) => s.is_primary);
    const sessionId = primary?.id ?? sessions[0]?.id ?? null;
    if (sessionId) {
      switchToSession(taskId, sessionId, currentOldSessionId);
      replaceTaskUrl(taskId);
      return;
    }

    const switched = await prepareAndSwitchTask(
      taskId,
      store,
      switchToSession,
      params.setPreparingTaskId,
    );
    if (switched) {
      replaceTaskUrl(taskId);
      return;
    }

    const currentOldEnvId = currentOldSessionId
      ? (store.getState().environmentIdBySessionId[currentOldSessionId] ?? null)
      : null;
    finalizeNoSessionSelect(taskId, currentOldEnvId, {
      setActiveTask: params.setActiveTask,
      releaseLayoutToDefault,
      replaceTaskUrl,
    });
  });
}
