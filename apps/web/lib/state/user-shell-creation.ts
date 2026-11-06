import type { StoreApi } from "zustand";
import type { AppState } from "./store";
import type { UserShellInfo } from "./slices/session-runtime/types";
import { userShellScopeKey, type UserShellScope } from "./user-shell-scope";

/**
 * One ordinary-terminal creation operation per app store and task/environment
 * scope. Desktop auto-migration, mobile auto-create, and explicit creates can
 * all race for the same empty list; joining the pending operation prevents
 * duplicate persisted terminals.
 */
const pendingCreations = new WeakMap<StoreApi<AppState>, Map<string, Promise<UserShellInfo>>>();

function guardFor(store: StoreApi<AppState>): Map<string, Promise<UserShellInfo>> {
  let guard = pendingCreations.get(store);
  if (!guard) {
    guard = new Map();
    pendingCreations.set(store, guard);
  }
  return guard;
}

export function createOrdinaryShellOnce(
  store: StoreApi<AppState>,
  scope: UserShellScope & { taskId: string },
  create: () => Promise<UserShellInfo>,
): Promise<UserShellInfo> {
  const key = userShellScopeKey(scope);
  const guard = guardFor(store);
  const existing = guard.get(key);
  if (existing) return existing;
  const creation = create()
    .then((shell) => {
      // Publish to the captured cache even if the caller navigates away.
      store.getState().addUserShell(scope, shell);
      return shell;
    })
    .finally(() => {
      guard.delete(key);
    });
  guard.set(key, creation);
  return creation;
}
