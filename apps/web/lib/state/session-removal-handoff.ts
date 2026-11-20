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
type RemovalHandoff = HandoffOwner & { targetSessionId: string };
const completedByStore = new WeakMap<StoreApi<AppState>, RemovalHandoff>();

/** Surface activation requires the same scoped proof that selected its recipient. */
export function getProvenRemovalPredecessor(
  store: StoreApi<AppState>,
  taskId: string,
  targetSessionId: string,
): string | null {
  const proof = completedByStore.get(store);
  if (!proof || proof.taskId !== taskId || proof.targetSessionId !== targetSessionId) return null;
  const state = store.getState();
  return state.tasks.activeTaskId === taskId &&
    state.tasks.activeSessionId === targetSessionId &&
    state.taskRemoval.navigationRevision === proof.navigationRevision &&
    state.workflowSessionFocus === proof.workflowFocus &&
    stateReadScopeIdentity(state) === proof.scopeIdentity
    ? proof.previousSessionId
    : null;
}

export function recordRemovalHandoff(
  store: StoreApi<AppState>,
  pending: HandoffOwner,
  target: string,
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
