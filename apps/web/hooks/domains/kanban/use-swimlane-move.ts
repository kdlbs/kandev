"use client";

import { useCallback } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { moveTask } from "@/lib/api/domains/kanban-api";
import { qk } from "@/lib/query/keys";
import type { KanbanMultiData } from "@/lib/query/query-options/kanban";
import type { Task } from "@/components/kanban-card";
import type { MoveTaskError } from "@/hooks/use-drag-and-drop";
import type { KanbanState } from "@/lib/state/slices/kanban/types";

type KanbanTask = KanbanState["tasks"][number];

// ---------------------------------------------------------------------------
// Mutation input shape
// ---------------------------------------------------------------------------

type MoveSwimlaneInput = {
  task: Task;
  targetStepId: string;
  currentTasks: KanbanTask[];
  workflowId: string;
};

// ---------------------------------------------------------------------------
// Optimistic snapshot builder
// ---------------------------------------------------------------------------

function buildOptimisticTasks(
  tasks: KanbanTask[],
  taskId: string,
  targetStepId: string,
  nextPosition: number,
): KanbanTask[] {
  return tasks.map((t) =>
    t.id === taskId ? { ...t, workflowStepId: targetStepId, position: nextPosition } : t,
  );
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

/**
 * Handles drag-and-drop task moves within a swimlane. Ports the Zustand
 * optimistic update into a `useMutation` onMutate/onError/onSettled pattern.
 *
 * Preserved signature: `useSwimlaneMove(workflowId, opts)`.
 */
export function useSwimlaneMove(
  workflowId: string,
  opts: {
    onMoveError?: (error: MoveTaskError) => void;
  },
) {
  const queryClient = useQueryClient();

  const mutation = useMutation({
    mutationFn: ({ task, targetStepId, currentTasks }: MoveSwimlaneInput) => {
      const targetTasks = currentTasks
        .filter((t) => t.workflowStepId === targetStepId && t.id !== task.id)
        .sort((a, b) => a.position - b.position);
      const nextPosition = targetTasks.length;
      return moveTask(task.id, {
        workflow_id: workflowId,
        workflow_step_id: targetStepId,
        position: nextPosition,
      });
    },

    onMutate: async ({ task, targetStepId, currentTasks }: MoveSwimlaneInput) => {
      // Cancel in-flight refetches to avoid overwriting the optimistic update
      await queryClient.cancelQueries({ queryKey: qk.kanban.multi() });

      // Snapshot current cache for rollback
      const previousData = queryClient.getQueryData<KanbanMultiData>(qk.kanban.multi());

      const targetTasks = currentTasks
        .filter((t) => t.workflowStepId === targetStepId && t.id !== task.id)
        .sort((a, b) => a.position - b.position);
      const nextPosition = targetTasks.length;

      // Apply optimistic update
      queryClient.setQueryData<KanbanMultiData>(qk.kanban.multi(), (prev) => {
        if (!prev) return prev;
        const snap = prev.snapshots[workflowId];
        if (!snap) return prev;
        const tasks = buildOptimisticTasks(snap.tasks, task.id, targetStepId, nextPosition);
        return {
          ...prev,
          snapshots: { ...prev.snapshots, [workflowId]: { ...snap, tasks } },
        };
      });

      return { previousData };
    },

    onError: (error, { task }, context) => {
      // Rollback to pre-mutation snapshot
      if (context?.previousData) {
        queryClient.setQueryData(qk.kanban.multi(), context.previousData);
      }
      const message = error instanceof Error ? error.message : "Failed to move task";
      opts.onMoveError?.({
        message,
        taskId: task.id,
        sessionId: task.primarySessionId ?? null,
      });
    },

    onSettled: () => {
      // Let the WS bridge own freshness; only invalidate to resync if the
      // bridge hasn't already updated within the next render cycle.
      queryClient.invalidateQueries({ queryKey: qk.kanban.multi() });
    },
  });

  const moveTaskFn = useCallback(
    (task: Task, targetStepId: string) => {
      if (task.workflowStepId === targetStepId) return;

      // Read current tasks from the cache at call time
      const currentData = queryClient.getQueryData<KanbanMultiData>(qk.kanban.multi());
      const currentTasks = currentData?.snapshots[workflowId]?.tasks ?? [];

      mutation.mutate({ task, targetStepId, currentTasks, workflowId });
    },
    [workflowId, queryClient, mutation],
  );

  return { moveTask: moveTaskFn };
}
