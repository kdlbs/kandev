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
      onMoveStart?.();
      const state = store.getState();
      const snapshot = state.kanbanMulti.snapshots[workflowId];
      if (!snapshot) return;

      const originalTask = snapshot.tasks.find((t) => t.id === taskId);
      if (!originalTask) return;

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
        // Rollback only the moved task, and only if it still has the optimistic values
        const cur = store.getState().kanbanMulti.snapshots[workflowId];
        const curTask = cur?.tasks.find((t) => t.id === taskId);
        if (cur && curTask?.workflowStepId === targetStepId && curTask.position === nextPosition) {
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
        }
        console.error("Failed to move task:", error);
        onMoveError?.(error);
      }
    },
    [store, moveTaskById, onMoveError, onMoveStart],
  );
}
