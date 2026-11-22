import type { StoreApi } from "zustand";
import type { AppState } from "./store";
import { storeReadScopeIdentity } from "./shared-resource-reads";
import { workspaceRelativeFilePath } from "@/lib/workspace-file-path";
import type { OpenFileTab } from "@/lib/types/backend";

type Store = StoreApi<AppState>;
export type TabletFileOpen = {
  current: (file?: OpenFileTab) => boolean;
  finish: () => void;
};
type PendingOpen = {
  operation: TabletFileOpen;
  scope: string;
  environment: string;
  path: string;
  deleted: Set<string>;
};
const pendingOpens = new WeakMap<object, Map<string, PendingOpen>>();
const openListeners = new WeakMap<object, Set<() => void>>();
const selectionOwners = new WeakMap<object, Map<string, symbol>>();
function notifyOpen(store: object) {
  for (const listener of openListeners.get(store) ?? []) listener();
}
export function tabletSelectionOwner(store: object, sessionId: string) {
  return selectionOwners.get(store)?.get(sessionId);
}
export function beginTabletSelection(store: object, sessionId: string) {
  pendingOpens.get(store)?.get(sessionId)?.operation.finish();
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
export function subscribeTabletFileOpen(store: Store, listener: () => void) {
  let listeners = openListeners.get(store);
  if (!listeners) openListeners.set(store, (listeners = new Set()));
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}
export function hasPendingTabletFileOpen(store: Store, sessionId: string): boolean {
  return pendingOpens.get(store)?.get(sessionId)?.operation.current() ?? false;
}
export function captureTabletFileOpen(
  store: Store,
  sessionId: string,
  path: string,
  current: () => boolean,
): TabletFileOpen {
  const selection = beginTabletSelection(store, sessionId);
  const scope = storeReadScopeIdentity(store);
  const environment = fileScope(store, sessionId);
  const navigation = store.getState().taskRemoval.navigationRevision;
  const { activeTaskId, activeSessionId } = store.getState().tasks;
  const deleted = new Set<string>();
  let valid = true;
  const operation: TabletFileOpen = {
    current(file) {
      if (
        !valid ||
        !current() ||
        storeReadScopeIdentity(store) !== scope ||
        fileScope(store, sessionId) !== environment ||
        tabletSelectionOwner(store, sessionId) !== selection ||
        store.getState().taskRemoval.navigationRevision !== navigation ||
        store.getState().tasks.activeTaskId !== activeTaskId ||
        store.getState().tasks.activeSessionId !== activeSessionId
      )
        return false;
      for (const parent of deleted) {
        if (
          workspaceRelativeFilePath(path, parent) !== null ||
          (file?.resolvedPath && workspaceRelativeFilePath(file.resolvedPath, parent) !== null)
        )
          return false;
      }
      return true;
    },
    finish() {
      valid = false;
      if (pendingOpens.get(store)?.get(sessionId)?.operation !== operation) return;
      pendingOpens.get(store)!.delete(sessionId);
      notifyOpen(store);
    },
  };
  let pending = pendingOpens.get(store);
  if (!pending) pendingOpens.set(store, (pending = new Map()));
  pending.set(sessionId, { operation, scope, environment, path, deleted });
  notifyOpen(store);
  return operation;
}
const fileAdmissions = new WeakMap<OpenFileTab, TabletFileOpen>();
export function bindTabletFileOpen(file: OpenFileTab, operation: TabletFileOpen): void {
  fileAdmissions.set(file, operation);
}
export function canAdmitTabletFile(file: OpenFileTab): boolean {
  return fileAdmissions.get(file)?.current(file) ?? true;
}
export function finishTabletFileOpen(file: OpenFileTab): void {
  fileAdmissions.get(file)?.finish();
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
/** Mutation success retires overlapping reads and snapshots in the same authenticated workspace. */
export function captureTabletFileDeletion(store: Store, sessionId: string, path: string) {
  const scope = storeReadScopeIdentity(store);
  const environment = fileScope(store, sessionId);
  return () => {
    if (storeReadScopeIdentity(store) !== scope || fileScope(store, sessionId) !== environment)
      return;
    for (const pending of pendingOpens.get(store)?.values() ?? []) {
      if (pending.scope !== scope || pending.environment !== environment) continue;
      pending.deleted.add(path);
      if (workspaceRelativeFilePath(pending.path, path) !== null) pending.operation.finish();
    }
    for (const listener of deletionListeners.get(store) ?? []) {
      if (fileScope(store, listener.sessionId) === environment) listener.deleted(path);
    }
  };
}
