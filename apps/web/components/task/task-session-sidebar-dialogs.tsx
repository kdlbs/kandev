"use client";

import { TaskRenameDialog } from "./task-rename-dialog";
import { TaskArchiveConfirmDialog } from "./task-archive-confirm-dialog";
import { TaskDeleteConfirmDialog } from "./task-delete-confirm-dialog";

type Target = { id: string; title: string; executorType?: string | null } | null;

export type SidebarDialogsActions = {
  renamingTask: Target;
  setRenamingTask: (next: Target) => void;
  handleRenameSubmit: (newTitle: string) => Promise<void> | void;
  archivingTask: Target;
  setArchivingTask: (next: Target) => void;
  archivingTaskId: string | null;
  isArchiving: boolean;
  handleArchiveConfirm: (opts: { cascade: boolean }) => Promise<void> | void;
  deletingTask: Target;
  setDeletingTask: (next: Target) => void;
  isDeleting: boolean;
  handleDeleteConfirm: (opts: { cascade: boolean }) => Promise<void> | void;
};

export function SidebarDialogs({ actions }: { actions: SidebarDialogsActions }) {
  const {
    renamingTask,
    setRenamingTask,
    handleRenameSubmit,
    archivingTask,
    setArchivingTask,
    archivingTaskId,
    isArchiving,
    handleArchiveConfirm,
    deletingTask,
    setDeletingTask,
    isDeleting,
    handleDeleteConfirm,
  } = actions;
  return (
    <>
      <TaskRenameDialog
        open={renamingTask !== null}
        onOpenChange={(open) => {
          if (!open) setRenamingTask(null);
        }}
        currentTitle={renamingTask?.title ?? ""}
        onSubmit={handleRenameSubmit}
      />
      <TaskArchiveConfirmDialog
        open={archivingTask !== null}
        onOpenChange={(open) => {
          if (!open) setArchivingTask(null);
        }}
        taskTitle={archivingTask?.title ?? ""}
        taskId={archivingTask?.id}
        executorType={archivingTask?.executorType}
        isArchiving={isArchiving && archivingTask?.id === archivingTaskId}
        onConfirm={handleArchiveConfirm}
      />
      <TaskDeleteConfirmDialog
        open={deletingTask !== null}
        onOpenChange={(open) => {
          if (!open) setDeletingTask(null);
        }}
        taskTitle={deletingTask?.title ?? ""}
        taskId={deletingTask?.id}
        executorType={deletingTask?.executorType}
        isDeleting={isDeleting}
        onConfirm={handleDeleteConfirm}
      />
    </>
  );
}
