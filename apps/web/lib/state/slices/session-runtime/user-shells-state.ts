import type {
  SessionPollMode,
  SessionRuntimeSlice,
  SessionRuntimeSliceState,
  UserShellInfo,
} from "./types";
import {
  isOrdinaryUserShell,
  userShellScopeEnvironment,
  userShellScopeKey,
  type UserShellScope,
} from "@/lib/state/user-shell-scope";

type ImmerSet = (recipe: (draft: SessionRuntimeSliceState) => void) => void;

type UserShellActions = Pick<
  SessionRuntimeSlice,
  | "setUserShells"
  | "setUserShellsLoading"
  | "setUserShellsFailed"
  | "addUserShell"
  | "removeUserShell"
  | "updateUserShell"
  | "setSessionPollMode"
>;

function partitionUserShells(shells: UserShellInfo[]) {
  const ordinary: UserShellInfo[] = [];
  const unmanaged: UserShellInfo[] = [];
  for (const shell of shells) {
    if (isOrdinaryUserShell(shell)) ordinary.push(shell);
    else unmanaged.push(shell);
  }
  return { ordinary, unmanaged };
}

function withoutDismissed(
  shells: UserShellInfo[],
  dismissed: Record<string, true> | undefined,
): UserShellInfo[] {
  if (!dismissed) return shells;
  return shells.filter((shell) => !dismissed[shell.terminalId]);
}

function setUserShells(set: ImmerSet): UserShellActions["setUserShells"] {
  return (scope, shells, sharedGeneration) =>
    set((draft) => {
      if (!scope.environmentId) return;
      const key = userShellScopeKey(scope);
      const { ordinary, unmanaged } = partitionUserShells(shells);
      const environmentId = scope.environmentId;
      const previousGeneration = draft.userShells.sharedGeneration[environmentId] ?? 0;
      const appliesShared =
        sharedGeneration === undefined || sharedGeneration >= previousGeneration;
      if (appliesShared) {
        const envDismissed = draft.userShells.dismissedByEnvironmentId[environmentId];
        draft.userShells.byEnvironmentId[environmentId] = withoutDismissed(unmanaged, envDismissed);
        if (sharedGeneration !== undefined && sharedGeneration > previousGeneration) {
          draft.userShells.sharedGeneration[environmentId] = sharedGeneration;
        }
      }
      if (scope.taskId) {
        // A task-scoped list is the only authority for ordinary rows. A
        // taskless list cannot prove ownership, so its ordinary items are
        // discarded rather than adopted into a scope. Ordinary rows keep
        // their own scope even when the shared snapshot is stale.
        const dismissed = draft.userShells.dismissedByScope[key];
        draft.userShells.byScope[key] = withoutDismissed(ordinary, dismissed);
      }
      draft.userShells.loaded[key] = true;
      draft.userShells.listed[key] = true;
      draft.userShells.loading[key] = false;
    });
}

function setUserShellsLoading(set: ImmerSet): UserShellActions["setUserShellsLoading"] {
  return (scope, loading) =>
    set((draft) => {
      if (!scope.environmentId) return;
      const key = userShellScopeKey(scope);
      draft.userShells.loading[key] = loading;
      // A pending read is not successful evidence. Cached rows stay visible
      // for continuity, but selection and automatic creation wait for the
      // current read to complete.
      if (loading) draft.userShells.listed[key] = false;
    });
}

function setUserShellsFailed(set: ImmerSet): UserShellActions["setUserShellsFailed"] {
  return (scope) =>
    set((draft) => {
      if (!scope.environmentId) return;
      const key = userShellScopeKey(scope);
      // Settled, but not a successful list: cached rows survive while
      // creation and selection authority are withdrawn until the next
      // successful read.
      draft.userShells.loading[key] = false;
      draft.userShells.loaded[key] = true;
      draft.userShells.listed[key] = false;
    });
}

function addUserShell(set: ImmerSet): UserShellActions["addUserShell"] {
  return (scope, shell) =>
    set((draft) => {
      if (!scope.environmentId) return;
      const env = scope.environmentId;
      if (isOrdinaryUserShell(shell)) {
        if (!scope.taskId) return;
        const key = userShellScopeKey(scope);
        if (draft.userShells.dismissedByScope[key]?.[shell.terminalId]) return;
        const existing = draft.userShells.byScope[key] || [];
        if (!existing.some((s) => s.terminalId === shell.terminalId)) {
          draft.userShells.byScope[key] = [...existing, shell];
        }
        return;
      }
      if (draft.userShells.dismissedByEnvironmentId[env]?.[shell.terminalId]) return;
      const existing = draft.userShells.byEnvironmentId[env] || [];
      if (!existing.some((s) => s.terminalId === shell.terminalId)) {
        draft.userShells.byEnvironmentId[env] = [...existing, shell];
      }
    });
}

function removeUserShell(set: ImmerSet): UserShellActions["removeUserShell"] {
  return (scope, terminalId) =>
    set((draft) => {
      if (!scope.environmentId) return;
      const key = userShellScopeKey(scope);
      const env = scope.environmentId;
      (draft.userShells.dismissedByScope[key] ??= {})[terminalId] = true;
      (draft.userShells.dismissedByEnvironmentId[env] ??= {})[terminalId] = true;
      const scopedRows = draft.userShells.byScope[key];
      if (scopedRows) {
        draft.userShells.byScope[key] = scopedRows.filter((s) => s.terminalId !== terminalId);
      }
      const unmanagedRows = draft.userShells.byEnvironmentId[env];
      if (unmanagedRows) {
        draft.userShells.byEnvironmentId[env] = unmanagedRows.filter(
          (s) => s.terminalId !== terminalId,
        );
      }
    });
}

function updateUserShell(set: ImmerSet): UserShellActions["updateUserShell"] {
  return (scope, terminalId, patch) =>
    set((draft) => {
      if (!scope.environmentId) return;
      const key = userShellScopeKey(scope);
      const env = scope.environmentId;
      const scopedRows = draft.userShells.byScope[key];
      if (scopedRows) {
        draft.userShells.byScope[key] = scopedRows.map((s) =>
          s.terminalId === terminalId ? { ...s, ...patch } : s,
        );
      }
      const unmanagedRows = draft.userShells.byEnvironmentId[env];
      if (unmanagedRows?.some((s) => s.terminalId === terminalId)) {
        draft.userShells.byEnvironmentId[env] = unmanagedRows.map((s) =>
          s.terminalId === terminalId ? { ...s, ...patch } : s,
        );
      }
    });
}

export function buildUserShellActions(set: ImmerSet): UserShellActions {
  return {
    setUserShells: setUserShells(set),
    setUserShellsLoading: setUserShellsLoading(set),
    setUserShellsFailed: setUserShellsFailed(set),
    addUserShell: addUserShell(set),
    removeUserShell: removeUserShell(set),
    updateUserShell: updateUserShell(set),
    setSessionPollMode: (sessionId: string, mode: SessionPollMode) =>
      set((draft) => {
        draft.sessionPollMode.bySessionId[sessionId] = mode;
      }),
  };
}

/** Drops every ordinary scope, tombstone, and flag for a removed environment. */
export function purgeUserShellEnvironment(
  userShells: SessionRuntimeSliceState["userShells"],
  envKey: string,
) {
  delete userShells.byEnvironmentId[envKey];
  delete userShells.dismissedByEnvironmentId[envKey];
  delete userShells.sharedGeneration[envKey];
  for (const key of Object.keys(userShells.byScope)) {
    if (userShellScopeEnvironment(key) === envKey) delete userShells.byScope[key];
  }
  for (const key of Object.keys(userShells.dismissedByScope)) {
    if (userShellScopeEnvironment(key) === envKey) delete userShells.dismissedByScope[key];
  }
  for (const map of [userShells.loading, userShells.loaded, userShells.listed]) {
    for (const key of Object.keys(map)) {
      if (userShellScopeEnvironment(key) === envKey) delete map[key];
    }
  }
}

/**
 * Moves taskless shell scopes off the session-id fallback key. Task-owned
 * scopes never use a session id as their environment, and taskless data is
 * never promoted into an owned scope.
 */
export function migrateUserShellScopes(
  draft: SessionRuntimeSliceState,
  sessionId: string,
  environmentId: string,
) {
  const sessionKey = userShellScopeKey({ environmentId: sessionId, taskId: null });
  const envKey = userShellScopeKey({ environmentId, taskId: null });
  const migrateScoped = <T>(store: Record<string, T>) => {
    if (!(sessionKey in store)) return;
    if (!(envKey in store)) store[envKey] = store[sessionKey];
    delete store[sessionKey];
  };
  migrateScoped(draft.userShells.byScope);
  migrateScoped(draft.userShells.dismissedByScope);
  migrateScoped(draft.userShells.loading);
  migrateScoped(draft.userShells.loaded);
  migrateScoped(draft.userShells.listed);
}

export type { UserShellScope };
