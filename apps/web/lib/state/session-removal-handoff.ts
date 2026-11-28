import type { StoreApi } from "zustand";
import type { AppState } from "./store";
import { stateReadScopeIdentity } from "./shared-resource-reads";

type HandoffOwner = {
  taskId: string;
  previousSessionId: string;
  scopeIdentity: string;
  navigationRevision: number | undefined;
  workflowFocus: AppState["workflowSessionFocus"];
};
type RemovalHandoff = HandoffOwner & { targetSessionId: string | null };
const completedByStore = new WeakMap<StoreApi<AppState>, RemovalHandoff>();

/** Selection and empty-slot rendering share one scoped removal receipt. */
export function getProvenRemovalPredecessor(
  store: StoreApi<AppState>,
  taskId: string,
  targetSessionId: string | null,
): string | null {
  const proof = completedByStore.get(store);
  if (!proof || proof.taskId !== taskId || proof.targetSessionId !== targetSessionId) return null;
  const state = store.getState();
  // Passive display protection ends at selection/navigation, not workflow cancellation.
  return state.tasks.activeTaskId === taskId &&
    state.tasks.activeSessionId === targetSessionId &&
    state.taskRemoval.navigationRevision === proof.navigationRevision &&
    stateReadScopeIdentity(state) === proof.scopeIdentity
    ? proof.previousSessionId
    : null;
}

export function recordRemovalHandoff(
  store: StoreApi<AppState>,
  pending: HandoffOwner,
  target: string | null,
  removed: boolean,
): void {
  const previous = completedByStore.get(store);
  const continuing =
    previous?.taskId === pending.taskId &&
    previous.targetSessionId === pending.previousSessionId &&
    previous.scopeIdentity === pending.scopeIdentity &&
    previous.navigationRevision === pending.navigationRevision &&
    previous.workflowFocus === pending.workflowFocus;
  const predecessor = removed ? pending.previousSessionId : previous?.previousSessionId;
  if (!predecessor || (!removed && !continuing)) {
    completedByStore.delete(store);
    return;
  }
  completedByStore.set(store, {
    taskId: pending.taskId,
    previousSessionId: predecessor,
    targetSessionId: target,
    scopeIdentity: pending.scopeIdentity,
    navigationRevision: pending.navigationRevision,
    workflowFocus: pending.workflowFocus,
  });
}
