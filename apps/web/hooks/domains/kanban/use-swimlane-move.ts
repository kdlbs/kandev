import { useCallback } from "react";
import { useAppStoreApi } from "@/components/state-provider";
import { useTaskActions } from "@/hooks/use-task-actions";
import type { Task } from "@/components/kanban-card";
import type { MoveTaskError } from "@/hooks/use-drag-and-drop";
import type { KanbanState } from "@/lib/state/slices/kanban/types";

export function useSwimlaneMove(
  workflowId: string,
  opts: {
    onMoveError?: (error: MoveTaskError) => void;
  },
) {
  const store = useAppStoreApi();
  const { moveTaskById } = useTaskActions();

  const moveTask = useCallback(
    async (task: Task, targetStepId: string) => {
      if (task.workflowStepId === targetStepId) return;

      const state = store.getState();
      const snapshot = state.kanbanMulti.snapshots[workflowId];
      if (!snapshot) return;

      const targetTasks = snapshot.tasks
        .filter(
          (t: KanbanState["tasks"][number]) =>
            t.workflowStepId === targetStepId && t.id !== task.id,
        )
        .sort(
          (a: KanbanState["tasks"][number], b: KanbanState["tasks"][number]) =>
            a.position - b.position,
        );
      const nextPosition = targetTasks.length;

      const originalTasks = snapshot.tasks;

      // Optimistic update
      state.setWorkflowSnapshot(workflowId, {
        ...snapshot,
        tasks: snapshot.tasks.map((t: KanbanState["tasks"][number]) =>
          t.id === task.id ? { ...t, workflowStepId: targetStepId, position: nextPosition } : t,
        ),
      });

      try {
        await moveTaskById(task.id, {
          workflow_id: workflowId,
          workflow_step_id: targetStepId,
          position: nextPosition,
        });
        // Backend handles on_enter actions (auto_start_agent, plan_mode, etc.)
        // via the task.moved event → orchestrator processOnEnter()
      } catch (error) {
        // Rollback
        const currentSnapshot = store.getState().kanbanMulti.snapshots[workflowId];
        if (currentSnapshot) {
          store.getState().setWorkflowSnapshot(workflowId, {
            ...currentSnapshot,
            tasks: originalTasks,
          });
        }
        const message = error instanceof Error ? error.message : "Failed to move task";
        opts.onMoveError?.({
          message,
          taskId: task.id,
          sessionId: task.primarySessionId ?? null,
        });
      }
    },
    [workflowId, store, moveTaskById, opts],
  );

  return { moveTask };
}
