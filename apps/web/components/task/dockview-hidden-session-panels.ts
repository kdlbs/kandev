import type { DockviewApi } from "dockview-react";
import { useDockviewStore } from "@/lib/state/dockview-store";
import {
  getEnvHiddenSessions,
  setEnvHiddenSessionOwner,
  setEnvHiddenSessions,
} from "@/lib/env-hidden-sessions";

const hiddenByApi = new WeakMap<DockviewApi, Map<string | null, Set<string>>>();

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

export function hideSessionPanel(api: DockviewApi, sessionId: string, taskId?: string): void {
  const envId = useDockviewStore.getState().currentLayoutEnvId;
  if (envId && taskId) setEnvHiddenSessionOwner(envId, sessionId, taskId);
  const hidden = hiddenSessionIdsFor(api);
  hidden.add(sessionId);
  persist(api, hidden);
  const panel = api.getPanel(`session:${sessionId}`);
  if (panel) api.removePanel(panel);
}

export function clearHiddenSessionPanel(api: DockviewApi, sessionId: string): void {
  const hidden = hiddenSessionIdsFor(api);
  hidden.delete(sessionId);
  persist(api, hidden);
}
