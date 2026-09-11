"use client";

import { useMemo } from "react";
import { KanbanCardContextMenu } from "@/components/kanban-card-context-menu";
import { useKanbanCardMenus } from "@/components/kanban-card-menu";
import { PipelineDialogs } from "./graph2-task-pipeline-dialogs";
import { excludeOrphanFromMoveMenu } from "./graph2-task-pipeline-step-run";
import { PipelineRow } from "./graph2-task-pipeline-row";
import { resolveTaskRepositoryChips } from "@/components/kanban-card-repositories";
import type { Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";
import type { KanbanExternalLinkAvailability } from "@/components/kanban-external-link-availability";
import type { Repository } from "@/lib/types/http";

export {
  computeAnchorScrollDelta,
  excludeOrphanFromMoveMenu,
  getStepAdjacency,
  getStepAdjacencyForStep,
  getStepMoveTargets,
} from "./graph2-task-pipeline-step-run";
export type { StepAdjacency, StepMoveTargets } from "./graph2-task-pipeline-step-run";

export type Graph2TaskPipelineProps = {
  task: Task;
  steps: WorkflowStep[];
  moveTargetSteps: WorkflowStep[];
  workspaceId: string | null;
  externalLinkAvailability: KanbanExternalLinkAvailability;
  repositories: Repository[];
  onMoveTask: (task: Task, targetStepId: string) => void;
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onEditTask?: (task: Task) => void;
  onDeleteTask: (
    task: Task,
    opts?: { cascade?: boolean; discardWorktreeChanges?: boolean },
  ) => void;
  onArchiveTask?: (
    task: Task,
    opts?: { cascade?: boolean; discardWorktreeChanges?: boolean },
  ) => void;
  isMoving?: boolean;
  isDeleting?: boolean;
  isArchiving?: boolean;
  isSelected?: boolean;
  selectedIds?: Set<string>;
  onToggleSelect?: (taskId: string) => void;
  onRangeSelect?: (taskId: string) => void;
  isMultiSelectMode?: boolean;
};

export function Graph2TaskPipeline({
  task,
  steps,
  moveTargetSteps,
  workspaceId,
  externalLinkAvailability,
  repositories,
  onMoveTask,
  onPreviewTask,
  onEditTask,
  onDeleteTask,
  onArchiveTask,
  isMoving,
  isDeleting,
  isArchiving,
  isSelected,
  selectedIds,
  onToggleSelect,
  onRangeSelect,
  isMultiSelectMode,
}: Graph2TaskPipelineProps) {
  const currentStepIndex = useMemo(
    () => steps.findIndex((s) => s.id === task.workflowStepId),
    [steps, task.workflowStepId],
  );
  const repositoryChips = useMemo(
    () => resolveTaskRepositoryChips(task, repositories),
    [task, repositories],
  );
  const menuMoveTargetSteps = useMemo(
    () => excludeOrphanFromMoveMenu(moveTargetSteps),
    [moveTargetSteps],
  );

  const menu = useKanbanCardMenus({
    task,
    workspaceId,
    externalLinkAvailability,
    steps: menuMoveTargetSteps,
    isDeleting,
    isArchiving,
    isMoving,
    isSelected,
    selectedIds,
    onEdit: onEditTask,
    onDelete: onDeleteTask,
    onArchive: onArchiveTask,
    onMove: onMoveTask,
  });

  const row = (
    <PipelineRow
      task={task}
      steps={steps}
      moveTargetSteps={moveTargetSteps}
      repositoryChips={repositoryChips}
      currentStepIndex={currentStepIndex}
      menu={menu}
      onMoveTask={onMoveTask}
      onPreviewTask={onPreviewTask}
      onToggleSelect={onToggleSelect}
      onRangeSelect={onRangeSelect}
      isMoving={isMoving}
      isDeleting={isDeleting}
      isArchiving={isArchiving}
      isSelected={isSelected}
      isMultiSelectMode={isMultiSelectMode}
    />
  );

  return (
    <>
      <div ref={menu.detachAnchorRef} className="w-full">
        {isMultiSelectMode ? (
          row
        ) : (
          <KanbanCardContextMenu entries={menu.contextMenuEntries}>{row}</KanbanCardContextMenu>
        )}
      </div>
      <PipelineDialogs
        task={task}
        workspaceId={workspaceId}
        repositories={repositories}
        menu={menu}
        isDeleting={isDeleting}
        isArchiving={isArchiving}
        onDeleteTask={onDeleteTask}
        onArchiveTask={onArchiveTask}
      />
    </>
  );
}
