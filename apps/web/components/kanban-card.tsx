"use client";

import { useDraggable } from "@dnd-kit/core";
import { KanbanCardContextMenu } from "@/components/kanban-card-context-menu";
import { KanbanCardShell } from "@/components/kanban-card-content";
import { useActiveWorkspaceRepositories } from "@/components/kanban-card-repositories";
export { resolveTaskRepositoryChips } from "@/components/kanban-card-repositories";
import {
  KanbanCardDialogs,
  useKanbanCardMenus,
  type KanbanCardMenuState,
} from "@/components/kanban-card-menu";
export { buildPluginMenuContext } from "@/components/kanban-card-menu";
import { useAppStore } from "@/components/state-provider";
import { TaskArchiveConfirmation } from "@/components/task/task-archive-confirmation";
import { TaskDetachConfirmationSurface } from "@/components/task/task-detach-confirm-dialog";
import type { KanbanExternalLinkAvailability } from "./kanban-external-link-availability";
import { useTaskWorkflowMove } from "@/hooks/use-task-workflow-move";
import { useTaskMultiSelectStore } from "@/hooks/use-task-multi-select";
import type { TaskActionOptions } from "@/hooks/use-task-actions";
import { useDetachTask } from "@/hooks/use-detach-task";
import { useUpdateTaskPriority } from "@/hooks/use-update-task-priority";
import { useTaskMenuDialogState } from "@/hooks/use-task-menu-dialog-state";
import { type TaskPriority } from "@/lib/types/http";
import type { PluginTaskMenuContext } from "@/lib/plugins/types";
import { usePluginRegistry } from "@/lib/plugins/registry";
import type { Task, RepositoryChip, WorkflowStep, KanbanPresentation } from "./kanban-card-types";

export type { Task, RepositoryChip, WorkflowStep, KanbanPresentation };

export interface KanbanCardProps {
  task: Task;
  workspaceId: string | null;
  presentation?: KanbanPresentation;
  externalLinkAvailability: KanbanExternalLinkAvailability;
  /** Display labels and hover paths of every repository linked to the task, primary first. */
  repositoryChips?: RepositoryChip[];
  onClick?: (task: Task) => void;
  onEdit?: (task: Task) => void;
  onDelete?: (task: Task, opts?: TaskActionOptions) => void;
  onArchive?: (task: Task, opts?: TaskActionOptions) => void;
  onOpenFullPage?: (task: Task) => void;
  onMove?: (task: Task, targetStepId: string) => void;
  steps?: WorkflowStep[];
  showMaximizeButton?: boolean;
  isDeleting?: boolean;
  isArchiving?: boolean;
  isSelected?: boolean;
  selectedIds?: Set<string>;
  onToggleSelect?: (taskId: string) => void;
  /** Shift-click range select within this card's column. */
  onRangeSelect?: (taskId: string) => void;
  isMultiSelectMode?: boolean;
}

/**
 * Cmd/Ctrl-click toggles a single card; Shift-click range-selects within the
 * column; either modifier enters multi-select mode without the toggle button.
 * A plain click toggles while in multi-select mode, otherwise previews/opens.
 */
/** @internal Exported for unit testing the four-branch click dispatch. */
export function dispatchKanbanCardClick(
  e: React.MouseEvent,
  taskId: string,
  task: Task,
  handlers: {
    onToggleSelect?: (taskId: string) => void;
    onRangeSelect?: (taskId: string) => void;
    onClick?: (task: Task) => void;
    isMultiSelectMode?: boolean;
  },
): void {
  // Only intercept a modifier click when the matching handler is wired, so a
  // card rendered without selection handlers still opens on Cmd/Shift click.
  if ((e.metaKey || e.ctrlKey) && handlers.onToggleSelect) {
    e.preventDefault();
    handlers.onToggleSelect(taskId);
    return;
  }
  if (e.shiftKey && handlers.onRangeSelect) {
    e.preventDefault();
    handlers.onRangeSelect(taskId);
    return;
  }
  if (handlers.isMultiSelectMode && handlers.onToggleSelect) {
    handlers.onToggleSelect(taskId);
    return;
  }
  handlers.onClick?.(task);
}

function KanbanCardFrame({
  task,
  presentation,
  repositoryChips,
  draggable,
  menu,
  isPreviewed,
  isSelected,
  isMultiSelectMode,
  showMaximizeButton,
  isDeleting,
  isArchiving,
  onArchive,
  onClick,
  onToggleSelect,
  onOpenFullPage,
}: Pick<
  KanbanCardProps,
  | "task"
  | "presentation"
  | "repositoryChips"
  | "isSelected"
  | "isMultiSelectMode"
  | "showMaximizeButton"
  | "isDeleting"
  | "isArchiving"
  | "onArchive"
  | "onToggleSelect"
  | "onOpenFullPage"
> & {
  draggable: ReturnType<typeof useDraggable>;
  menu: KanbanCardMenuState;
  isPreviewed: boolean;
  onClick: (e: React.MouseEvent) => void;
}) {
  return (
    <>
      <div ref={menu.detachAnchorRef} className="w-full">
        <KanbanCardContextMenu entries={menu.contextMenuEntries}>
          <KanbanCardShell
            task={task}
            repositoryChips={repositoryChips}
            attributes={draggable.attributes}
            listeners={draggable.listeners}
            setNodeRef={draggable.setNodeRef}
            transform={draggable.transform}
            isDragging={draggable.isDragging}
            isPreviewed={isPreviewed}
            isSelected={isSelected}
            isMultiSelectMode={isMultiSelectMode}
            showMaximizeButton={showMaximizeButton}
            isDeleting={isDeleting}
            isArchiving={isArchiving}
            menuEntries={menu.dropdownMenuEntries}
            menuTriggerRef={menu.detachFocusReturnRef}
            onClick={onClick}
            onCheckboxClick={(e) => {
              e.stopPropagation();
              onToggleSelect?.(task.id);
            }}
            onOpenFullPage={onOpenFullPage}
          />
        </KanbanCardContextMenu>
      </div>
      <TaskDetachConfirmationSurface
        taskId={task.id}
        open={menu.showDetachConfirm}
        anchorRef={menu.detachAnchorRef}
        focusReturnRef={menu.detachFocusReturnRef}
        taskTitle={task.title}
        sharesParentWorkspace={task.workspaceMode === "inherit_parent"}
        onOpenChange={menu.setShowDetachConfirm}
        onConfirm={menu.handleDetachConfirm}
      />
      <TaskArchiveConfirmation
        open={menu.showArchiveConfirm}
        anchorRef={menu.archiveAnchorRef}
        focusReturnRef={menu.archiveFocusReturnRef}
        taskTitle={task.title}
        taskId={task.id}
        executorType={task.primaryExecutorType}
        isArchiving={isArchiving}
        forceDialog={presentation === "mobile"}
        onOpenChange={menu.setShowArchiveConfirm}
        onConfirm={({ cascade }) => onArchive?.(task, { cascade })}
      />
    </>
  );
}

export function KanbanCard({
  task,
  workspaceId,
  presentation = "desktop",
  externalLinkAvailability,
  repositoryChips,
  onClick,
  onEdit,
  onDelete,
  onArchive,
  onOpenFullPage,
  onMove,
  steps,
  showMaximizeButton = false,
  isDeleting,
  isArchiving,
  isSelected,
  selectedIds,
  onToggleSelect,
  onRangeSelect,
  isMultiSelectMode,
}: KanbanCardProps) {
  const draggable = useDraggable({
    id: task.id,
    disabled: isMultiSelectMode,
  });
  const isPreviewed = useAppStore((state) => state.kanbanPreviewedTaskId === task.id);
  const repositories = useActiveWorkspaceRepositories();
  const menu = useKanbanCardMenus({
    task,
    workspaceId,
    presentation,
    externalLinkAvailability,
    steps,
    isDeleting,
    isArchiving,
    isSelected,
    selectedIds,
    onEdit,
    onDelete,
    onArchive,
    onMove,
  });

  const handleClick = (e: React.MouseEvent) =>
    dispatchKanbanCardClick(e, task.id, task, {
      onToggleSelect,
      onRangeSelect,
      onClick,
      isMultiSelectMode,
    });

  return (
    <>
      <KanbanCardFrame
        task={task}
        presentation={presentation}
        repositoryChips={repositoryChips}
        draggable={draggable}
        menu={menu}
        isPreviewed={isPreviewed}
        isSelected={isSelected}
        isMultiSelectMode={isMultiSelectMode}
        showMaximizeButton={showMaximizeButton}
        isDeleting={isDeleting}
        isArchiving={isArchiving}
        onArchive={onArchive}
        onClick={handleClick}
        onToggleSelect={onToggleSelect}
        onOpenFullPage={onOpenFullPage}
      />
      <KanbanCardDialogs
        task={task}
        workspaceId={workspaceId}
        repositories={repositories}
        menu={menu}
        isDeleting={isDeleting}
        onDelete={onDelete}
      />
    </>
  );
}
