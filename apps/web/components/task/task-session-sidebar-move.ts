import { useCallback } from "react";
import { useAppStoreApi } from "@/components/state-provider";
import { useTaskActions } from "@/hooks/use-task-actions";

type StoreApi = ReturnType<typeof useAppStoreApi>;

/** Optimistically moves a task to a workflow step, rolling back on rejection. */
export function useMoveToStep(
  store: StoreApi,
  onMoveStart?: () => void,
  onMoveError?: (error: unknown) => void,
) {
  const { moveTaskById } = useTaskActions();

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

      const targetTasks = snapshot.tasks
        .filter((t) => t.workflowStepId === targetStepId && t.id !== taskId)
        .sort((a, b) => a.position - b.position);
      const nextPosition = targetTasks.length;

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
      } catch (error) {
        console.error("Failed to move task:", error);
        // The optimistic values still being in place is what makes this the
        // move the store is showing. Once a newer move has overwritten them
        // this rejection describes an abandoned request, so it must neither
        // roll back nor paint a banner over the newer move's outcome.
        const cur = store.getState().kanbanMulti.snapshots[workflowId];
        const curTask = cur?.tasks.find((t) => t.id === taskId);
        const isCurrentMove =
          !!cur && curTask?.workflowStepId === targetStepId && curTask.position === nextPosition;
        if (!isCurrentMove) return;

        store.getState().setWorkflowSnapshot(workflowId, {
          ...cur,
          tasks: cur.tasks.map((t) =>
            t.id === taskId
              ? {
                  ...t,
                  workflowStepId: originalTask.workflowStepId,
                  position: originalTask.position,
                }
              : t,
          ),
        });
        onMoveError?.(error);
      }
    },
    [store, moveTaskById, onMoveError, onMoveStart],
  );
}
