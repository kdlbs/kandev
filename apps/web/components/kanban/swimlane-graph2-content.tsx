"use client";

import { useMemo, useState } from "react";
import { useSwimlaneMove } from "@/hooks/domains/kanban/use-swimlane-move";
import { Graph2TaskPipeline } from "./graph2-task-pipeline";
import { ORPHAN_STEP, ORPHAN_STEP_ID, remapOrphanTasks } from "./swimlane-kanban-content";
import type { ViewContentProps } from "@/lib/kanban/view-registry";
import type { Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";
import { useTranslation } from "react-i18next";
import { areAllEmptyStepsAutoHidden } from "@/lib/kanban/auto-hide-empty-columns";
import { compareStepOrder } from "@/lib/kanban/task-order";

/**
 * Pipeline row order: each step's contiguous run of rows in step-list order,
 * then within one step the full AC.1 total order
 * (REQ-TASKS-KANBAN-TASK-REORDERING-001.2, .38) — position is not by itself a
 * total order, and ties are the ship-time norm for tasks that arrived
 * together.
 */
export function sortGraph2Tasks(displayTasks: Task[], displaySteps: WorkflowStep[]): Task[] {
  const stepIndex = new Map(displaySteps.map((step, index) => [step.id, index]));
  return [...displayTasks].sort((a, b) => {
    const aStepIdx = stepIndex.get(a.workflowStepId) ?? -1;
    const bStepIdx = stepIndex.get(b.workflowStepId) ?? -1;
    if (aStepIdx !== bStepIdx) return aStepIdx - bStepIdx;
    return compareStepOrder(a, b);
  });
}

export function getGraph2DisplayState(
  tasks: Task[],
  steps: WorkflowStep[],
  orphanStepTitle: string,
): { displayTasks: Task[]; displaySteps: WorkflowStep[] } {
  const stepIds = new Set(steps.map((step) => step.id));
  const { tasks: displayTasks, hasOrphans } = remapOrphanTasks(tasks, stepIds, ORPHAN_STEP_ID);
  return {
    displayTasks,
    displaySteps: hasOrphans ? [...steps, { ...ORPHAN_STEP, title: orphanStepTitle }] : steps,
  };
}

export function SwimlaneGraph2Content({
  workflowId,
  steps,
  moveTargetSteps,
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
  const { t } = useTranslation();
  const { moveTask } = useSwimlaneMove(workflowId, {
    onMoveError,
  });
  const [movingTaskId, setMovingTaskId] = useState<string | null>(null);
  const { displayTasks, displaySteps } = useMemo(
    () => getGraph2DisplayState(tasks, steps, t("kanban:needsReassignment")),
    [tasks, steps, t],
  );
  const pipelineMoveTargetSteps = useMemo(() => {
    const orphan = displaySteps.find((step) => step.id === ORPHAN_STEP_ID);
    return orphan ? [...moveTargetSteps, orphan] : moveTargetSteps;
  }, [displaySteps, moveTargetSteps]);

  const sortedTasks = useMemo(
    () => sortGraph2Tasks(displayTasks, displaySteps),
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
        <div
          className="text-xs text-muted-foreground text-center py-4"
          data-testid={
            areAllEmptyStepsAutoHidden(steps, moveTargetSteps)
              ? "pipeline-auto-hidden-empty-state"
              : undefined
          }
        >
          {areAllEmptyStepsAutoHidden(steps, moveTargetSteps)
            ? t("kanban:allEmptyStepsAutoHidden")
            : t("kanban:noTasks")}
        </div>
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
            moveTargetSteps={pipelineMoveTargetSteps}
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
