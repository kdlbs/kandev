"use client";

import { useMemo, useState } from "react";
import { useSwimlaneMove } from "@/hooks/domains/kanban/use-swimlane-move";
import { Graph2TaskPipeline } from "./graph2-task-pipeline";
import { ORPHAN_STEP, ORPHAN_STEP_ID, remapOrphanTasks } from "./swimlane-kanban-content";
import type { ViewContentProps } from "@/lib/kanban/view-registry";
import type { Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";

export function getGraph2DisplayState(
  tasks: Task[],
  steps: WorkflowStep[],
): { displayTasks: Task[]; displaySteps: WorkflowStep[] } {
  const stepIds = new Set(steps.map((step) => step.id));
  const { tasks: displayTasks, hasOrphans } = remapOrphanTasks(tasks, stepIds, ORPHAN_STEP_ID);
  return {
    displayTasks,
    displaySteps: hasOrphans ? [...steps, ORPHAN_STEP] : steps,
  };
}

export function SwimlaneGraph2Content({
  workflowId,
  steps,
  tasks,
  onPreviewTask,
  onOpenTask,
  onDeleteTask,
  onArchiveTask,
  onMoveError,
  deletingTaskId,
  archivingTaskId,
  selectedIds,
  onToggleSelect,
  isMultiSelectMode,
}: ViewContentProps) {
  const { moveTask } = useSwimlaneMove(workflowId, {
    onMoveError,
  });
  const [movingTaskId, setMovingTaskId] = useState<string | null>(null);
  const { displayTasks, displaySteps } = useMemo(
    () => getGraph2DisplayState(tasks, steps),
    [tasks, steps],
  );

  const sortedTasks = useMemo(
    () =>
      [...displayTasks].sort((a, b) => {
        const aStepIdx = displaySteps.findIndex((c) => c.id === a.workflowStepId);
        const bStepIdx = displaySteps.findIndex((c) => c.id === b.workflowStepId);
        if (aStepIdx !== bStepIdx) return aStepIdx - bStepIdx;
        return (a.position ?? 0) - (b.position ?? 0);
      }),
    [displayTasks, displaySteps],
  );

  const handleMoveTask = async (task: (typeof tasks)[number], targetStepId: string) => {
    setMovingTaskId(task.id);
    try {
      await moveTask(task, targetStepId);
    } finally {
      setMovingTaskId(null);
    }
  };

  if (displayTasks.length === 0) {
    return (
      <div className="px-3 pb-3">
        <div className="text-xs text-muted-foreground text-center py-4">No tasks</div>
      </div>
    );
  }

  return (
    <div className="px-3 pb-3 overflow-x-auto">
      <div className="space-y-1">
        {sortedTasks.map((task) => (
          <Graph2TaskPipeline
            key={task.id}
            task={task}
            steps={displaySteps}
            onMoveTask={handleMoveTask}
            onPreviewTask={onPreviewTask}
            onOpenTask={onOpenTask}
            onDeleteTask={onDeleteTask}
            onArchiveTask={onArchiveTask}
            isMoving={movingTaskId === task.id}
            isDeleting={deletingTaskId === task.id}
            isArchiving={archivingTaskId === task.id}
            isSelected={selectedIds?.has(task.id)}
            onToggleSelect={onToggleSelect}
            isMultiSelectMode={isMultiSelectMode}
          />
        ))}
      </div>
    </div>
  );
}
