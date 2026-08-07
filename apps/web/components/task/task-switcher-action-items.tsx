"use client";

import { IconArchive, IconLoader, IconSubtask, IconTrash, IconUnlink } from "@tabler/icons-react";
import { ContextMenuItem, ContextMenuSeparator } from "@kandev/ui/context-menu";
import { useTranslation } from "react-i18next";

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
  const { t } = useTranslation();
  if (actingOnSelection && onBulkArchive) {
    const count = actingIds.length;
    return (
      <ContextMenuItem disabled={disabled} onSelect={() => onBulkArchive(actingIds)}>
        <IconArchive className="mr-2 h-4 w-4" />
        {count > 1 ? t("task:archiveTasks", { count }) : t("task:archive")}
      </ContextMenuItem>
    );
  }
  if (!onArchiveTask) return null;
  return (
    <ContextMenuItem disabled={disabled} onSelect={() => onArchiveTask(taskId)}>
      <IconArchive className="mr-2 h-4 w-4" />
      {t("task:archive")}
    </ContextMenuItem>
  );
}

export function TaskCreateSubtaskItem({
  task,
  disabled,
  onCreateSubtask,
}: {
  task: { id: string; title: string };
  disabled?: boolean;
  onCreateSubtask?: (taskId: string, taskTitle: string) => void;
}) {
  const { t } = useTranslation();
  if (!onCreateSubtask) return null;
  return (
    <ContextMenuItem
      data-testid="task-context-create-subtask"
      disabled={disabled}
      onSelect={() => onCreateSubtask(task.id, task.title)}
    >
      <IconSubtask className="mr-2 h-4 w-4" />
      {t("task:createSubtask")}
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
  const { t } = useTranslation();
  if (!task.parentTaskId || !onDetachTask) return null;
  return (
    <ContextMenuItem
      data-testid="task-context-detach"
      disabled={disabled}
      onSelect={() => onDetachTask(task.id)}
    >
      <IconUnlink className="mr-2 h-4 w-4" />
      {t("task:detachFromParent")}
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
  const { t } = useTranslation();
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
        {t("task:delete")}
      </ContextMenuItem>
    </>
  );
}
