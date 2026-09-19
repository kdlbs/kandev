import type { DockviewApi } from "dockview-react";
import { useDockviewStore } from "@/lib/state/dockview-store";
import {
  getEnvHiddenSessions,
  getEnvHiddenSessionRecords,
  setEnvHiddenSessionOwner,
  setEnvHiddenSessions,
} from "@/lib/env-hidden-sessions";
import type { TaskId } from "@/lib/types/http";

const hiddenByApi = new WeakMap<DockviewApi, Map<string | null, Set<string>>>();
const hideListenersByApi = new WeakMap<DockviewApi, Set<(sessionId: string) => void>>();

export function onDidHideSessionPanel(
  api: DockviewApi,
  listener: (sessionId: string) => void,
): { dispose: () => void } {
  let listeners = hideListenersByApi.get(api);
  if (!listeners) {
    listeners = new Set();
    hideListenersByApi.set(api, listeners);
  }
  listeners.add(listener);
  return { dispose: () => listeners.delete(listener) };
}

export function hiddenSessionIdsFor(api: DockviewApi): Set<string> {
  const envId = useDockviewStore.getState().currentLayoutEnvId;
  let byEnv = hiddenByApi.get(api);
  if (!byEnv) {
    byEnv = new Map();
    hiddenByApi.set(api, byEnv);
  }
  const current = byEnv.get(envId);
  if (current) return current;
  const loaded = new Set(envId ? getEnvHiddenSessions(envId) : []);
  byEnv.set(envId, loaded);
  return loaded;
}

function persist(api: DockviewApi, hidden: Set<string>) {
  const envId = useDockviewStore.getState().currentLayoutEnvId;
  if (envId) setEnvHiddenSessions(envId, [...hidden]);
}

function shouldPruneHiddenSession(args: {
  sessionId: string;
  owners: Map<string, string>;
  sessions: Record<string, { task_id?: TaskId }>;
  sessionsByTask?: Record<string, Array<{ id: string }>>;
  loadedByTask?: Record<string, boolean>;
}): boolean {
  const ownerTaskId = args.owners.get(args.sessionId) || args.sessions[args.sessionId]?.task_id;
  if (!ownerTaskId || args.loadedByTask?.[ownerTaskId] !== true) return false;
  return !args.sessionsByTask?.[ownerTaskId]?.some((session) => session.id === args.sessionId);
}

/**
 * Retire a hidden record only when its owning task has loaded and no longer
 * contains that session. A shared environment can contain unrelated tasks.
 */
export function pruneHiddenSessionIds(
  api: DockviewApi,
  store: () => {
    taskSessionsByTask?: {
      itemsByTaskId?: Record<string, Array<{ id: string }>>;
      loadedByTaskId?: Record<string, boolean>;
    };
    taskSessions?: { items?: Record<string, { task_id?: TaskId }> };
  },
): void {
  const envId = useDockviewStore.getState().currentLayoutEnvId;
  if (!envId) return;
  const hidden = hiddenSessionIdsFor(api);
  const state = store();
  const records = new Map(
    getEnvHiddenSessionRecords(envId).map((record) => [record.sessionId, record.taskId]),
  );
  const sessionsByTask = state.taskSessionsByTask?.itemsByTaskId;
  const loadedByTask = state.taskSessionsByTask?.loadedByTaskId;
  const sessions = state.taskSessions?.items ?? {};
  let changed = false;
  for (const sessionId of hidden) {
    if (
      !shouldPruneHiddenSession({
        sessionId,
        owners: records,
        sessions,
        sessionsByTask,
        loadedByTask,
      })
    ) {
      continue;
    }
    hidden.delete(sessionId);
    changed = true;
  }
  if (changed) persist(api, hidden);
}

export function hideSessionPanel(api: DockviewApi, sessionId: string, taskId?: string): void {
  const envId = useDockviewStore.getState().currentLayoutEnvId;
  if (envId && taskId) setEnvHiddenSessionOwner(envId, sessionId, taskId);
  const hidden = hiddenSessionIdsFor(api);
  hidden.add(sessionId);
  persist(api, hidden);
  hideListenersByApi.get(api)?.forEach((listener) => listener(sessionId));
  const panel = api.getPanel(`session:${sessionId}`);
  if (panel) api.removePanel(panel);
}

export function clearHiddenSessionPanel(api: DockviewApi, sessionId: string): void {
  const hidden = hiddenSessionIdsFor(api);
  hidden.delete(sessionId);
  persist(api, hidden);
}
