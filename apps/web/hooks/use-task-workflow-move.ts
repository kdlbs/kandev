"use client";

import { useCallback } from "react";
import { bulkMoveSelectedTasks } from "@/lib/api";
import { useToast } from "@/components/toast-provider";

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : "Failed to move task";
}

function movedTitle(movedCount: number) {
  if (movedCount === 1) return "Moved task to workflow";
  return `Moved ${movedCount} tasks to workflow`;
}

function movedDescription(movedCount: number) {
  return movedCount === 1
    ? "Switch to the destination workflow to see it."
    : "Switch to the destination workflow to see them.";
}

export function useTaskWorkflowMove() {
  const { toast } = useToast();

  return useCallback(
    async (taskIds: string[], targetWorkflowId: string, targetStepId: string) => {
      const ids = [...new Set(taskIds.filter(Boolean))];
      if (ids.length === 0) return;
      try {
        const result = await bulkMoveSelectedTasks({
          task_ids: ids,
          target_workflow_id: targetWorkflowId,
          target_step_id: targetStepId,
        });
        toast({
          title: movedTitle(result.moved_count),
          description: movedDescription(result.moved_count),
          variant: "success",
        });
      } catch (error) {
        toast({
          title: "Failed to move task",
          description: errorMessage(error),
          variant: "error",
        });
        throw error;
      }
    },
    [toast],
  );
}
