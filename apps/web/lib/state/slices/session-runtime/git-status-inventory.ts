import type { SessionRuntimeSliceState } from "./types";

/** Returns the environment's inventory generation, or 0 before the first one. */
export function gitStatusInventoryGeneration(
  state: Pick<SessionRuntimeSliceState, "gitStatus">,
  environmentId: string,
): number {
  return state.gitStatus.inventoryByEnvironment?.[environmentId]?.generation ?? 0;
}

/**
 * Reports whether a streaming status belongs to a scope that a validated
 * inventory retired. With unknown inventory (null scopes) nothing is retired,
 * so an attachment can establish a new current root again.
 */
export function isRetiredGitStatusScope(
  state: Pick<SessionRuntimeSliceState, "gitStatus">,
  environmentId: string,
  repositoryName: string | undefined,
): boolean {
  const inventory = state.gitStatus.inventoryByEnvironment?.[environmentId];
  if (!inventory?.scopes) return false;
  return (inventory.retiredScopes ?? []).includes(repositoryName ?? "");
}

/**
 * Reports whether a streaming status belongs to the validated current scope
 * set. Unknown inventory accepts everything so pre-existing behavior and
 * initial hydration are unchanged; retired scopes are rejected.
 */
export function isCurrentGitStatusScope(
  state: Pick<SessionRuntimeSliceState, "gitStatus">,
  environmentId: string,
  repositoryName: string | undefined,
): boolean {
  return !isRetiredGitStatusScope(state, environmentId, repositoryName);
}

/** True when inventory is known but this repository is neither current nor retired. */
export function isUnknownGitStatusScope(
  state: Pick<SessionRuntimeSliceState, "gitStatus">,
  environmentId: string,
  repositoryName: string | undefined,
): boolean {
  const inventory = state.gitStatus.inventoryByEnvironment?.[environmentId];
  if (!inventory?.scopes) return false;
  const scope = repositoryName ?? "";
  return !inventory.scopes.includes(scope) && !(inventory.retiredScopes ?? []).includes(scope);
}

/** Advances the generation once and marks the scope set unknown. */
export function invalidateGitStatusInventory(
  state: SessionRuntimeSliceState,
  environmentId: string,
): void {
  if (!environmentId) return;
  const byEnvironment = (state.gitStatus.inventoryByEnvironment ??= {});
  const current = byEnvironment[environmentId];
  byEnvironment[environmentId] = {
    generation: (current?.generation ?? 0) + 1,
    scopes: null,
    retiredScopes: current?.retiredScopes,
  };
}

function pruneScopedRecord(
  record: Record<string, unknown> | undefined,
  scopes: ReadonlySet<string>,
): boolean {
  if (!record) return false;
  let changed = false;
  for (const key of Object.keys(record)) {
    if (scopes.has(key)) continue;
    delete record[key];
    changed = true;
  }
  return changed;
}

function pruneEnvironmentMap(
  record: Record<string, unknown> | undefined,
  scopes: ReadonlySet<string>,
  removeParent: () => void,
): void {
  if (pruneScopedRecord(record, scopes) && Object.keys(record ?? {}).length === 0) {
    removeParent();
  }
}

function pruneLegacyMirror(
  state: SessionRuntimeSliceState,
  environmentId: string,
  scopes: ReadonlySet<string>,
): void {
  const legacy = state.gitStatus.byEnvironmentId[environmentId];
  if (legacy && !scopes.has(legacy.repository_name ?? "")) {
    delete state.gitStatus.byEnvironmentId[environmentId];
  }
}

function pruneObsoleteEnvironmentFailure(
  state: SessionRuntimeSliceState,
  environmentId: string,
): void {
  // A validated aggregate supersedes a settled environment-level failure.
  // `pending` is the only in-flight state; `unavailable` keeps a request_id
  // from the attempt that produced it and still must not outlive the inventory.
  const environmentRefresh = state.gitStatus.refreshByEnvironmentId?.[environmentId];
  if (environmentRefresh?.state === "unavailable") {
    delete state.gitStatus.refreshByEnvironmentId?.[environmentId];
  }
}

function pruneEnvironmentScopes(
  state: SessionRuntimeSliceState,
  environmentId: string,
  scopes: ReadonlySet<string>,
): void {
  pruneEnvironmentMap(
    state.gitStatus.byEnvironmentRepo[environmentId],
    scopes,
    () => delete state.gitStatus.byEnvironmentRepo[environmentId],
  );
  pruneEnvironmentMap(
    state.gitStatusDisplay.byEnvironmentRepo[environmentId],
    scopes,
    () => delete state.gitStatusDisplay.byEnvironmentRepo[environmentId],
  );
  pruneEnvironmentMap(
    state.gitStatus.refreshByEnvironmentRepo?.[environmentId],
    scopes,
    () => delete state.gitStatus.refreshByEnvironmentRepo?.[environmentId],
  );
  pruneLegacyMirror(state, environmentId, scopes);
  pruneObsoleteEnvironmentFailure(state, environmentId);
}

/**
 * Applies a validated aggregate inventory for one environment. A non-matching
 * generation means the response belongs to a replaced workspace and must not
 * retire anything.
 */
export function reconcileGitStatusInventory(
  state: SessionRuntimeSliceState,
  environmentId: string,
  generation: number,
  scopes: readonly string[],
): boolean {
  if (!environmentId) return false;
  const byEnvironment = (state.gitStatus.inventoryByEnvironment ??= {});
  if ((byEnvironment[environmentId]?.generation ?? 0) !== generation) return false;
  const normalized = [...new Set(scopes)].sort((a, b) => a.localeCompare(b));
  const previous = byEnvironment[environmentId];
  const retired = new Set(previous?.retiredScopes ?? []);
  for (const scope of previous?.scopes ?? []) {
    if (!normalized.includes(scope)) retired.add(scope);
  }
  for (const scope of normalized) retired.delete(scope);
  byEnvironment[environmentId] = {
    generation,
    scopes: normalized,
    retiredScopes: [...retired].sort((a, b) => a.localeCompare(b)),
  };
  pruneEnvironmentScopes(state, environmentId, new Set(normalized));
  return true;
}
