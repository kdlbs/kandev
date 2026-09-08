"use client";

import { KanbanCardDialogs, type KanbanCardMenuState } from "@/components/kanban-card-menu";
import { TaskArchiveConfirmation } from "@/components/task/task-archive-confirmation";
import { TaskDetachConfirmationSurface } from "@/components/task/task-detach-confirm-dialog";
import type { Task } from "@/components/kanban-card";
import type { Repository } from "@/lib/types/http";

/** The row's dialogs/confirmations, sourced from the shared menu module. */
export function PipelineDialogs({
  task,
  workspaceId,
  repositories,
  menu,
  isDeleting,
  isArchiving,
  onDeleteTask,
  onArchiveTask,
}: {
  task: Task;
  workspaceId: string | null;
  repositories: Repository[];
  menu: KanbanCardMenuState;
  isDeleting?: boolean;
  isArchiving?: boolean;
  onDeleteTask: (
    task: Task,
    opts?: { cascade?: boolean; discardWorktreeChanges?: boolean },
  ) => void;
  onArchiveTask?: (
    task: Task,
    opts?: { cascade?: boolean; discardWorktreeChanges?: boolean },
  ) => void;
}) {
  return (
    <>
      <KanbanCardDialogs
        task={task}
        workspaceId={workspaceId}
        repositories={repositories}
        menu={menu}
        isDeleting={isDeleting}
        onDelete={onDeleteTask}
      />
      <TaskDetachConfirmationSurface
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
        onOpenChange={menu.setShowArchiveConfirm}
        onConfirm={({ cascade }) => onArchiveTask?.(task, { cascade })}
      />
    </>
  );
}
