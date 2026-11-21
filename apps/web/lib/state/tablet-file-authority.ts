import type { StoreApi } from "zustand";
import type { AppState } from "./store";
import { storeReadScopeIdentity } from "./shared-resource-reads";
import type { OpenFileTab } from "@/lib/types/backend";

type Store = StoreApi<AppState>;
const selectionOwners = new WeakMap<object, Map<string, symbol>>();
export function tabletSelectionOwner(store: object, sessionId: string) {
  return selectionOwners.get(store)?.get(sessionId);
}
export function beginTabletSelection(store: object, sessionId: string) {
  let owners = selectionOwners.get(store);
  if (!owners) selectionOwners.set(store, (owners = new Map()));
  const owner = Symbol();
  owners.set(sessionId, owner);
  return owner;
}
function fileScope(store: Store, sessionId: string): string {
  const state = store.getState();
  const environment =
    state.environmentIdBySessionId[sessionId] ??
    state.taskSessions.items[sessionId]?.task_environment_id;
  return environment ? `environment:${environment}` : `session:${sessionId}`;
}
export function captureTabletFileOpen(store: Store, sessionId: string, current: () => boolean) {
  const selection = beginTabletSelection(store, sessionId);
  const scope = storeReadScopeIdentity(store);
  const environment = fileScope(store, sessionId);
  const navigation = store.getState().taskRemoval.navigationRevision;
  const { activeTaskId, activeSessionId } = store.getState().tasks;
  return () =>
    current() &&
    storeReadScopeIdentity(store) === scope &&
    fileScope(store, sessionId) === environment &&
    tabletSelectionOwner(store, sessionId) === selection &&
    store.getState().taskRemoval.navigationRevision === navigation &&
    store.getState().tasks.activeTaskId === activeTaskId &&
    store.getState().tasks.activeSessionId === activeSessionId;
}
const fileAdmissions = new WeakMap<OpenFileTab, () => boolean>();
export function bindTabletFileOpen(file: OpenFileTab, current: () => boolean): void {
  fileAdmissions.set(file, current);
}
export function canAdmitTabletFile(file: OpenFileTab): boolean {
  return fileAdmissions.get(file)?.() ?? true;
}

type DeletionListener = { sessionId: string; deleted: (path: string) => void };
const deletionListeners = new WeakMap<Store, Set<DeletionListener>>();
export function subscribeTabletFileDeletion(
  store: Store,
  sessionId: string,
  deleted: (path: string) => void,
) {
  let listeners = deletionListeners.get(store);
  if (!listeners) deletionListeners.set(store, (listeners = new Set()));
  const listener = { sessionId, deleted };
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}
/** Mutation success only retires snapshots from the same authenticated workspace. */
export function captureTabletFileDeletion(store: Store, sessionId: string, path: string) {
  const scope = storeReadScopeIdentity(store);
  const environment = fileScope(store, sessionId);
  return () => {
    if (storeReadScopeIdentity(store) !== scope || fileScope(store, sessionId) !== environment)
      return;
    for (const listener of deletionListeners.get(store) ?? []) {
      if (fileScope(store, listener.sessionId) === environment) listener.deleted(path);
    }
  };
}
