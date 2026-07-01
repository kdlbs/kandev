import { useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTaskActions } from "@/hooks/use-task-actions";
import type { Task } from "@/components/kanban-card";
import type { MoveTaskError } from "@/lib/kanban/move-task-error";
import {
  updateWorkflowSnapshotQuery,
  workflowSnapshotQueryDataForWorkflow,
} from "@/lib/query/workflow-snapshot-cache";

export function useSwimlaneMove(
  workflowId: string,
  opts: {
    onMoveError?: (error: MoveTaskError) => void;
  },
) {
  const queryClient = useQueryClient();
  const { moveTaskById } = useTaskActions();

  const moveTask = useCallback(
    async (task: Task, targetStepId: string) => {
      if (task.workflowStepId === targetStepId) return;

      const snapshot = workflowSnapshotQueryDataForWorkflow(queryClient, workflowId);
      if (!snapshot) return;

      const targetTasks = snapshot.tasks
        .filter((item) => item.workflow_step_id === targetStepId && item.id !== task.id)
        .sort((a, b) => (a.position ?? 0) - (b.position ?? 0));
      const nextPosition = targetTasks.length;

      const originalSnapshot = snapshot;

      updateWorkflowSnapshotQuery(queryClient, workflowId, (current) => ({
        ...current,
        tasks: current.tasks.map((item) =>
          item.id === task.id
            ? { ...item, workflow_step_id: targetStepId, position: nextPosition }
            : item,
        ),
      }));

      try {
        await moveTaskById(task.id, {
          workflow_id: workflowId,
          workflow_step_id: targetStepId,
          position: nextPosition,
        });
        // Backend handles on_enter actions (auto_start_agent, plan_mode, etc.)
        // via the task.moved event → orchestrator processOnEnter()
      } catch (error) {
        updateWorkflowSnapshotQuery(queryClient, workflowId, () => originalSnapshot);
        const message = error instanceof Error ? error.message : "Failed to move task";
        opts.onMoveError?.({
          message,
          taskId: task.id,
          sessionId: task.primarySessionId ?? null,
        });
      }
    },
    [workflowId, queryClient, moveTaskById, opts],
  );

  return { moveTask };
}
