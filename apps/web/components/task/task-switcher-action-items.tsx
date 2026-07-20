"use client";

import { IconArchive, IconLoader, IconTrash, IconUnlink } from "@tabler/icons-react";
import { ContextMenuItem, ContextMenuSeparator } from "@kandev/ui/context-menu";

export function TaskArchiveItem({
  taskId,
  actingIds,
  actingOnSelection,
  disabled,
  onArchiveTask,
  onBulkArchive,
}: {
  taskId: string;
  actingIds: string[];
  actingOnSelection: boolean;
  disabled?: boolean;
  onArchiveTask?: (taskId: string) => void;
  onBulkArchive?: (taskIds: string[]) => void;
}) {
  if (actingOnSelection && onBulkArchive) {
    const count = actingIds.length;
    return (
      <ContextMenuItem disabled={disabled} onSelect={() => onBulkArchive(actingIds)}>
        <IconArchive className="mr-2 h-4 w-4" />
        {count > 1 ? `Archive ${count} tasks` : "Archive"}
      </ContextMenuItem>
    );
  }
  if (!onArchiveTask) return null;
  return (
    <ContextMenuItem disabled={disabled} onSelect={() => onArchiveTask(taskId)}>
      <IconArchive className="mr-2 h-4 w-4" />
      Archive
    </ContextMenuItem>
  );
}

export function TaskDetachItem({
  task,
  disabled,
  onDetachTask,
}: {
  task: { id: string; parentTaskId?: string | null };
  disabled?: boolean;
  onDetachTask?: (taskId: string) => void;
}) {
  if (!task.parentTaskId || !onDetachTask) return null;
  return (
    <ContextMenuItem
      data-testid="task-context-detach"
      disabled={disabled}
      onSelect={() => onDetachTask(task.id)}
    >
      <IconUnlink className="mr-2 h-4 w-4" />
      Detach from parent
    </ContextMenuItem>
  );
}

export function TaskDeleteItem({
  taskId,
  isDeleting,
  onDeleteTask,
}: {
  taskId: string;
  isDeleting?: boolean;
  onDeleteTask?: (taskId: string) => void;
}) {
  if (!onDeleteTask) return null;
  return (
    <>
      <ContextMenuSeparator />
      <ContextMenuItem
        variant="destructive"
        disabled={isDeleting}
        onSelect={() => onDeleteTask(taskId)}
      >
        {isDeleting ? (
          <IconLoader className="mr-2 h-4 w-4 animate-spin" />
        ) : (
          <IconTrash className="mr-2 h-4 w-4" />
        )}
        Delete
      </ContextMenuItem>
    </>
  );
}
