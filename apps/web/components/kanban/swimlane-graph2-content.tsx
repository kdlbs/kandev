"use client";

import { useMemo } from "react";
import { useSwimlaneMove } from "@/hooks/domains/kanban/use-swimlane-move";
import { useTaskMoveGuard } from "@/hooks/domains/kanban/use-task-move-guard";
import { useAppStore } from "@/components/state-provider";
import { useActiveWorkspaceRepositories } from "@/components/kanban-card-repositories";
import { useKanbanExternalLinkAvailability } from "@/components/kanban-external-link-availability";
import { Graph2TaskPipeline } from "./graph2-task-pipeline";
import { ORPHAN_STEP, ORPHAN_STEP_ID, remapOrphanTasks } from "./swimlane-kanban-content";
import type { ViewContentProps } from "@/lib/kanban/view-registry";
import type { Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";
import { useTranslation } from "react-i18next";
import { areAllEmptyStepsAutoHidden } from "@/lib/kanban/auto-hide-empty-columns";

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

/**
 * AC-UI-PIPELINE-ROW-005.1: displayed-step-index ascending (a task with no
 * resolvable current step sorts after every task that has one), then
 * `position` ascending (absent treated as 0), then task id ascending.
 */
export function sortGraph2Tasks(tasks: Task[], displaySteps: WorkflowStep[]): Task[] {
  // A finite sentinel, not Number.POSITIVE_INFINITY: two no-resolvable-step
  // tasks must still subtract to a finite, sortable delta so their position/id
  // tiebreak below is reachable (Infinity - Infinity is NaN, which a sort
  // comparator cannot use to order a pair).
  const noStepSentinel = displaySteps.length;
  const stepIndex = (task: Task) => {
    const index = displaySteps.findIndex((step) => step.id === task.workflowStepId);
    return index === -1 ? noStepSentinel : index;
  };
  return [...tasks].sort((a, b) => {
    const stepDelta = stepIndex(a) - stepIndex(b);
    if (stepDelta !== 0) return stepDelta;
    const positionDelta = (a.position ?? 0) - (b.position ?? 0);
    if (positionDelta !== 0) return positionDelta;
    if (a.id < b.id) return -1;
    if (a.id > b.id) return 1;
    return 0;
  });
}

export function SwimlaneGraph2Content({
  workflowId,
  steps,
  moveTargetSteps,
  tasks,
  onPreviewTask,
  onOpenTask,
  onEditTask,
  onDeleteTask,
  onArchiveTask,
  onMoveError,
  deletingTaskId,
  archivingTaskId,
  selectedIds,
  onToggleSelect,
  onSelectRange,
  isMultiSelectMode,
}: ViewContentProps) {
  const { t } = useTranslation();
  const { moveTask } = useSwimlaneMove(workflowId, {
    onMoveError,
  });
  const { movingTaskIds, handleMoveTask } = useTaskMoveGuard(moveTask);
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const repositories = useActiveWorkspaceRepositories();
  const externalLinkAvailability = useKanbanExternalLinkAvailability(workspaceId);
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
  const orderedTaskIds = useMemo(() => sortedTasks.map((task) => task.id), [sortedTasks]);

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
            workspaceId={workspaceId}
            externalLinkAvailability={externalLinkAvailability}
            repositories={repositories}
            onMoveTask={handleMoveTask}
            onPreviewTask={onPreviewTask}
            onOpenTask={onOpenTask}
            onEditTask={onEditTask}
            onDeleteTask={onDeleteTask}
            onArchiveTask={onArchiveTask}
            isMoving={movingTaskIds.has(task.id)}
            isDeleting={deletingTaskId === task.id}
            isArchiving={archivingTaskId === task.id}
            isSelected={selectedIds?.has(task.id)}
            onToggleSelect={onToggleSelect}
            onRangeSelect={
              onSelectRange ? (taskId) => onSelectRange(taskId, orderedTaskIds) : undefined
            }
            isMultiSelectMode={isMultiSelectMode}
          />
        ))}
      </div>
    </div>
  );
}
