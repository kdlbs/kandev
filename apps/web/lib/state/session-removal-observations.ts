import type { StoreApi } from "zustand";
import type { AppState } from "./store";
import { stateReadScopeIdentity } from "./shared-resource-reads";

const observedByStore = new WeakMap<StoreApi<AppState>, { scope: string; ids: Set<string> }>();

/** Scoped unavailable observations exclude admission while absence proof is pending. */
export function observedRemovals(store: StoreApi<AppState>): Set<string> | undefined {
  const observed = observedByStore.get(store);
  return observed?.scope === stateReadScopeIdentity(store.getState()) ? observed.ids : undefined;
}

export function recordObservedSessionRemoval(
  store: StoreApi<AppState>,
  sessionId: string,
): boolean {
  const scope = stateReadScopeIdentity(store.getState());
  let observed = observedByStore.get(store);
  if (!observed || observed.scope !== scope) {
    observed = { scope, ids: new Set() };
    observedByStore.set(store, observed);
  }
  if (observed.ids.has(sessionId)) return false;
  observed.ids.add(sessionId);
  return true;
}
