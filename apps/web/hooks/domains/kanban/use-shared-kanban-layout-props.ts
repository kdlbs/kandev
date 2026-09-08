import { useMemo } from "react";
import type { Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";
import type { KanbanExternalLinkAvailability } from "@/components/kanban-external-link-availability";

export type SharedKanbanLayoutProps = {
  steps: WorkflowStep[];
  moveTargetSteps: WorkflowStep[];
  tasks: Task[];
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onEditTask: (task: Task) => void;
  onDeleteTask: (task: Task) => void;
  onArchiveTask?: (task: Task) => void;
  moveTaskToStep: (task: Task, targetStepId: string) => Promise<void>;
  showMaximizeButton?: boolean;
  deletingTaskId?: string | null;
  archivingTaskId?: string | null;
  selectedIds?: Set<string>;
  onToggleSelect?: (taskId: string) => void;
  onSelectRange?: (taskId: string, orderedIds: string[]) => void;
  isMultiSelectMode?: boolean;
  externalLinkAvailability: KanbanExternalLinkAvailability;
  temporaryStepIds: Set<string>;
  isDragging: boolean;
};

type SharedLayoutDragState = {
  renderedSteps: WorkflowStep[];
  moveTaskToStep: SharedKanbanLayoutProps["moveTaskToStep"];
  temporaryStepIds: Set<string>;
  activeTask: Task | null;
};

type SharedLayoutHookOptions = Omit<
  SharedKanbanLayoutProps,
  "steps" | "tasks" | "moveTaskToStep" | "temporaryStepIds" | "isDragging"
> & {
  drag: SharedLayoutDragState;
  displayTasks: Task[];
};

export function useSharedKanbanLayoutProps(
  options: SharedLayoutHookOptions,
): SharedKanbanLayoutProps {
  const { drag, displayTasks, externalLinkAvailability } = options;
  return useMemo(
    () => ({
      steps: drag.renderedSteps,
      moveTargetSteps: options.moveTargetSteps,
      tasks: displayTasks,
      onPreviewTask: options.onPreviewTask,
      onOpenTask: options.onOpenTask,
      onEditTask: options.onEditTask,
      onDeleteTask: options.onDeleteTask,
      onArchiveTask: options.onArchiveTask,
      moveTaskToStep: drag.moveTaskToStep,
      showMaximizeButton: options.showMaximizeButton,
      deletingTaskId: options.deletingTaskId,
      archivingTaskId: options.archivingTaskId,
      selectedIds: options.selectedIds,
      onToggleSelect: options.onToggleSelect,
      onSelectRange: options.onSelectRange,
      isMultiSelectMode: options.isMultiSelectMode,
      externalLinkAvailability,
      temporaryStepIds: drag.temporaryStepIds,
      isDragging: !!drag.activeTask,
    }),
    [
      drag.renderedSteps,
      drag.moveTaskToStep,
      drag.temporaryStepIds,
      drag.activeTask,
      options.moveTargetSteps,
      displayTasks,
      options.onPreviewTask,
      options.onOpenTask,
      options.onEditTask,
      options.onDeleteTask,
      options.onArchiveTask,
      options.showMaximizeButton,
      options.deletingTaskId,
      options.archivingTaskId,
      options.selectedIds,
      options.onToggleSelect,
      options.onSelectRange,
      options.isMultiSelectMode,
      externalLinkAvailability,
    ],
  );
}
