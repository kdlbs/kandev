import type { UserShellInfo, UserShellsState } from "./slices/session-runtime/types";

/**
 * Identity of an ordinary terminal result set.
 *
 * Different tasks can share one execution environment, so an environment
 * alone cannot namespace ordinary terminals: two tasks would read and write
 * the same rows. `taskId` is `null` for a taskless read, which is encoded
 * distinctly from every task so it can never populate an owned scope.
 */
export type UserShellScope = {
  environmentId: string;
  taskId: string | null;
};

/** Stable tuple key for a scope. Do not build this string anywhere else. */
export function userShellScopeKey(scope: UserShellScope): string {
  return JSON.stringify([scope.environmentId, scope.taskId]);
}

/** Builds a scope, or null when there is no environment to key on. */
export function makeUserShellScope(
  environmentId: string | null | undefined,
  taskId?: string | null,
): UserShellScope | null {
  if (!environmentId) return null;
  return { environmentId, taskId: taskId ?? null };
}

/** Environment encoded in a scope key produced by `userShellScopeKey`. */
export function userShellScopeEnvironment(key: string): string | null {
  try {
    const parsed: unknown = JSON.parse(key);
    return Array.isArray(parsed) && typeof parsed[0] === "string" ? parsed[0] : null;
  } catch {
    return null;
  }
}

/** Ordinary terminals are task-owned; every other kind is environment-shared. */
export function isOrdinaryUserShell(shell: Pick<UserShellInfo, "kind">): boolean {
  return shell.kind === "ordinary";
}

export function sameUserShellScope(
  left: UserShellScope | null,
  right: UserShellScope | null,
): boolean {
  if (!left || !right) return left === right;
  return left.environmentId === right.environmentId && left.taskId === right.taskId;
}

/**
 * Minimal store shape needed to resolve the active task/environment scope.
 * Kept structural so components, hooks, and plain helpers can use the same
 * predicate without importing the whole app-state type.
 */
export type ActiveUserShellState = {
  tasks?: { activeSessionId?: string | null; activeTaskId?: string | null } | null;
  environmentIdBySessionId: Record<string, string>;
  workspaceContextGeneration?: number;
  workspaces?: { activeId?: string | null } | null;
};

/** Scope of the task/environment the user is currently looking at. */
export function resolveActiveUserShellScope(state: ActiveUserShellState): UserShellScope | null {
  const sessionId = state.tasks?.activeSessionId ?? null;
  const environmentId = sessionId ? (state.environmentIdBySessionId[sessionId] ?? null) : null;
  return makeUserShellScope(environmentId, state.tasks?.activeTaskId ?? null);
}

/**
 * True when an asynchronous action still belongs to the visible scope. Late
 * results may update their captured cache, but must not publish panels or
 * local selection into a replacement task.
 */
export function isActiveUserShellScope(
  state: ActiveUserShellState,
  scope: UserShellScope | null,
): boolean {
  const active = resolveActiveUserShellScope(state);
  return (
    active !== null && scope !== null && userShellScopeKey(active) === userShellScopeKey(scope)
  );
}

/** Nested hydration merge: every shell namespace keeps its default maps. */
export function mergeUserShellsState(
  defaults: UserShellsState,
  initial?: Partial<UserShellsState> | null,
): UserShellsState {
  const source = initial ?? {};
  return {
    byScope: { ...defaults.byScope, ...source.byScope },
    byEnvironmentId: { ...defaults.byEnvironmentId, ...source.byEnvironmentId },
    dismissedByScope: { ...defaults.dismissedByScope, ...source.dismissedByScope },
    dismissedByEnvironmentId: {
      ...defaults.dismissedByEnvironmentId,
      ...source.dismissedByEnvironmentId,
    },
    loading: { ...defaults.loading, ...source.loading },
    loaded: { ...defaults.loaded, ...source.loaded },
    listed: { ...defaults.listed, ...source.listed },
    sharedGeneration: { ...defaults.sharedGeneration, ...source.sharedGeneration },
  };
}

/**
 * Identity captured when an asynchronous terminal action is admitted. A late
 * completion may update its captured cache, but may only publish panels or
 * local selection while this identity still matches the visible store.
 */
export type UserShellPublicationOwner = {
  scope: UserShellScope;
  workspaceId: string | null;
  workspaceContextGeneration: number;
};

export function captureUserShellPublicationOwner(
  state: ActiveUserShellState,
  scope: UserShellScope,
): UserShellPublicationOwner {
  return {
    scope,
    workspaceId: state.workspaces?.activeId ?? null,
    workspaceContextGeneration: state.workspaceContextGeneration ?? 0,
  };
}

export function isActiveUserShellPublicationOwner(
  state: ActiveUserShellState,
  owner: UserShellPublicationOwner,
): boolean {
  if (!isActiveUserShellScope(state, owner.scope)) return false;
  if ((state.workspaces?.activeId ?? null) !== owner.workspaceId) return false;
  return (state.workspaceContextGeneration ?? 0) === owner.workspaceContextGeneration;
}
