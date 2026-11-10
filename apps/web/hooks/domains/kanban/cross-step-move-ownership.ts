import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { TaskOverview } from "@/lib/state/slices/task-overview-types";

type AppStore = StoreApi<AppState>;
type PendingMove = {
  workflowId: string;
  taskId: string;
  targetStepId: string;
  originalStepId: string;
  workspaceId?: string;
  readId: string;
  retained: boolean;
  status: "pending" | "failed" | "succeeded";
  reclaimable: boolean;
  previous?: PendingMove;
};
const pendingByStore = new WeakMap<AppStore, Map<string, PendingMove>>();

function hasContext(state: AppState, move: PendingMove): boolean {
  const task = state.kanbanMulti.snapshots[move.workflowId]?.tasks.find(
    (t) => t.id === move.taskId,
  );
  const read = state.taskOverview.reads[move.readId];
  return Boolean(
    move.retained &&
    task?.workflowId === move.workflowId &&
    task.workspaceId === move.workspaceId &&
    read?.scope === state.taskOverview.scope &&
    read.changes[move.taskId] !== null,
  );
}

function ownsStep(state: AppState, move: PendingMove): boolean {
  if (!hasContext(state, move)) return false;
  const task = state.kanbanMulti.snapshots[move.workflowId].tasks.find(
    (t) => t.id === move.taskId,
  )!;
  const change = state.taskOverview.reads[move.readId].changes[move.taskId];
  return (
    task.workflowStepId === move.targetStepId &&
    !(change && Object.hasOwn(change, "workflowStepId"))
  );
}

function writeStep(store: AppStore, workflowId: string, taskId: string, stepId: string) {
  const snapshot = store.getState().kanbanMulti.snapshots[workflowId];
  // Only the stage is optimistic; the server owns arrival position.
  store.getState().setWorkflowSnapshot(workflowId, {
    ...snapshot,
    tasks: snapshot.tasks.map((task) =>
      task.id === taskId ? { ...task, workflowStepId: stepId } : task,
    ),
  });
}

function rollbackFailedMove(store: AppStore, move: PendingMove): PendingMove | undefined {
  const state = store.getState();
  move.reclaimable = hasContext(state, move);
  move.status = "failed";
  if (!ownsStep(state, move)) return;
  let step = move.originalStepId;
  let previous = move.previous;
  while (previous?.status === "failed" && previous.reclaimable) {
    step = previous.originalStepId;
    previous = previous.previous;
  }
  writeStep(store, move.workflowId, move.taskId, step);
  if (previous?.status !== "pending" || !hasContext(store.getState(), previous)) return;
  // Removing rejected local optimism restores only an unexpired predecessor claim.
  store.getState().finishTaskOverviewRead(previous.readId);
  previous.readId = store.getState().beginTaskOverviewRead();
  return previous;
}

export function beginCrossStepMove(
  store: AppStore,
  workflowId: string,
  taskId: string,
  targetStepId: string,
  originalTask: TaskOverview,
) {
  let pending = pendingByStore.get(store);
  if (!pending) {
    pending = new Map();
    pendingByStore.set(store, pending);
  }
  const key = JSON.stringify([workflowId, taskId]);
  const previous = pending.get(key);
  const linked =
    previous?.status === "pending" && ownsStep(store.getState(), previous) ? previous : undefined;
  writeStep(store, workflowId, taskId, targetStepId);
  const move: PendingMove = {
    workflowId,
    taskId,
    targetStepId,
    originalStepId: originalTask.workflowStepId,
    workspaceId: originalTask.workspaceId,
    readId: store.getState().beginTaskOverviewRead(),
    retained: true,
    status: "pending",
    reclaimable: false,
    previous: linked,
  };
  pending.set(key, move);
  const unsubscribe = store.subscribe((next) => {
    if (!next.kanbanMulti.snapshots[workflowId]?.tasks.some((task) => task.id === taskId))
      move.retained = false;
  });
  const retireCurrent = (replacement?: PendingMove) => {
    if (pending.get(key) !== move) return;
    if (replacement) pending.set(key, replacement);
    else pending.delete(key);
  };
  return {
    succeed: () => {
      move.status = "succeeded";
      retireCurrent();
    },
    rollback: () => retireCurrent(rollbackFailedMove(store, move)),
    dispose: () => {
      unsubscribe();
      store.getState().finishTaskOverviewRead(move.readId);
      retireCurrent();
      if (pending.size === 0 && pendingByStore.get(store) === pending) pendingByStore.delete(store);
    },
  };
}
