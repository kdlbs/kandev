import { useCallback, useRef } from "react";
import { useAppStoreApi } from "@/components/state-provider";
import { useTaskActions } from "@/hooks/use-task-actions";

type StoreApi = ReturnType<typeof useAppStoreApi>;
type TaskPosition = { workflowStepId: string; position: number };
type MoveState = {
  committedTask: TaskPosition;
  latestRequestId: number;
  latestTarget: TaskPosition;
  latestFailed: boolean;
  latestError?: unknown;
  pendingRequestIds: Set<number>;
};

type TaskSnapshot = {
  tasks: Array<{ id: string; workflowStepId: string; position: number }>;
};

type MoveCompletionContext = {
  store: StoreApi;
  moveStates: Map<string, MoveState>;
  moveKey: string;
  taskId: string;
  workflowId: string;
  requestId: number;
  onMoveError?: (error: unknown) => void;
};

function taskMatchesPosition(
  snapshot: TaskSnapshot | undefined,
  taskId: string,
  target: TaskPosition,
): boolean {
  const task = snapshot?.tasks.find((item) => item.id === taskId);
  return task?.workflowStepId === target.workflowStepId && task.position === target.position;
}

function setTaskPosition(
  store: StoreApi,
  workflowId: string,
  taskId: string,
  target: TaskPosition,
) {
  const snapshot = store.getState().kanbanMulti.snapshots[workflowId];
  if (!snapshot) return;
  store.getState().setWorkflowSnapshot(workflowId, {
    ...snapshot,
    tasks: snapshot.tasks.map((task) =>
      task.id === taskId
        ? { ...task, workflowStepId: target.workflowStepId, position: target.position }
        : task,
    ),
  });
}

function completeMoveSuccess({
  store,
  moveStates,
  moveKey,
  taskId,
  workflowId,
  requestId,
  target,
  onMoveError,
}: MoveCompletionContext & { target: TaskPosition }) {
  const state = moveStates.get(moveKey);
  if (!state) return;
  state.pendingRequestIds.delete(requestId);
  if (state.latestRequestId === requestId) state.latestFailed = false;
  state.committedTask = target;
  if (state.pendingRequestIds.size > 0) return;

  const snapshot = store.getState().kanbanMulti.snapshots[workflowId];
  const isOptimisticState = taskMatchesPosition(snapshot, taskId, state.latestTarget);
  moveStates.delete(moveKey);
  if (!isOptimisticState) return;

  setTaskPosition(store, workflowId, taskId, state.committedTask);
  if (state.latestFailed) onMoveError?.(state.latestError);
}

function completeMoveFailure({
  store,
  moveStates,
  moveKey,
  taskId,
  workflowId,
  requestId,
  error,
  onMoveError,
}: MoveCompletionContext & { error: unknown }) {
  const state = moveStates.get(moveKey);
  if (!state) return;
  state.pendingRequestIds.delete(requestId);
  if (state.latestRequestId === requestId) {
    state.latestFailed = true;
    state.latestError = error;
  }
  if (state.pendingRequestIds.size > 0) return;

  const snapshot = store.getState().kanbanMulti.snapshots[workflowId];
  const isCurrentMove = taskMatchesPosition(snapshot, taskId, state.latestTarget);
  moveStates.delete(moveKey);
  if (!isCurrentMove) return;

  setTaskPosition(store, workflowId, taskId, state.committedTask);
  if (state.latestFailed) onMoveError?.(state.latestError);
}

/** Optimistically moves a task to a workflow step, rolling back on rejection. */
export function useMoveToStep(
  store: StoreApi,
  onMoveStart?: () => void,
  onMoveError?: (error: unknown) => void,
) {
  const { moveTaskById } = useTaskActions();
  // The optimistic values alone cannot tell two moves apart when both target the
  // same step at the same computed position (a double tap, a rapid retry): the
  // second write is byte-identical to the first, so the first's rejection would
  // match and roll back a move that is still in flight. The generation says
  // which request the store is actually showing.
  //
  // Counted per task, not per hook: the sidebar calls this once and shares it
  // across every row, so a single counter would let a move of task B mark a
  // pending move of task A superseded and strand A in a step the backend
  // refused. Entries are never dropped, because reusing a generation while an
  // older request for that task is still in flight would recreate the very
  // collision the counter exists to prevent.
  const moveRequestsRef = useRef(new Map<string, number>());
  const moveStatesRef = useRef(new Map<string, MoveState>());

  return useCallback(
    async (taskId: string, workflowId: string, targetStepId: string) => {
      const state = store.getState();
      const snapshot = state.kanbanMulti.snapshots[workflowId];
      if (!snapshot) return;

      const originalTask = snapshot.tasks.find((t) => t.id === taskId);
      if (!originalTask) return;

      // Only signal the start once a move is actually going out: the guards
      // above return without touching the server, and clearing the banner on
      // the way past them would wipe a still-accurate message for nothing.
      onMoveStart?.();
      const moveKey = `${workflowId}:${taskId}`;
      const requestId = (moveRequestsRef.current.get(moveKey) ?? 0) + 1;
      moveRequestsRef.current.set(moveKey, requestId);

      const targetTasks = snapshot.tasks
        .filter((t) => t.workflowStepId === targetStepId && t.id !== taskId)
        .sort((a, b) => a.position - b.position);
      const nextPosition = targetTasks.length;

      const previousState = moveStatesRef.current.get(moveKey);
      const moveState =
        previousState && previousState.pendingRequestIds.size > 0
          ? previousState
          : {
              committedTask: {
                workflowStepId: originalTask.workflowStepId,
                position: originalTask.position,
              },
              latestRequestId: requestId,
              latestTarget: { workflowStepId: targetStepId, position: nextPosition },
              latestFailed: false,
              pendingRequestIds: new Set<number>(),
            };
      moveState.latestRequestId = requestId;
      moveState.latestTarget = { workflowStepId: targetStepId, position: nextPosition };
      moveState.latestFailed = false;
      moveState.latestError = undefined;
      moveState.pendingRequestIds.add(requestId);
      moveStatesRef.current.set(moveKey, moveState);

      // Optimistic update
      state.setWorkflowSnapshot(workflowId, {
        ...snapshot,
        tasks: snapshot.tasks.map((t) =>
          t.id === taskId ? { ...t, workflowStepId: targetStepId, position: nextPosition } : t,
        ),
      });

      try {
        await moveTaskById(taskId, {
          workflow_id: workflowId,
          workflow_step_id: targetStepId,
          position: nextPosition,
        });
        completeMoveSuccess({
          store,
          moveStates: moveStatesRef.current,
          moveKey,
          taskId,
          workflowId,
          requestId,
          target: { workflowStepId: targetStepId, position: nextPosition },
          onMoveError,
        });
      } catch (error) {
        console.error("Failed to move task:", error);
        completeMoveFailure({
          store,
          moveStates: moveStatesRef.current,
          moveKey,
          taskId,
          workflowId,
          requestId,
          error,
          onMoveError,
        });
      }
    },
    [store, moveTaskById, onMoveError, onMoveStart],
  );
}
