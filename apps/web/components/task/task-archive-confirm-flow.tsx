"use client";

import { TaskArchiveConfirmDialog } from "@/components/task/task-archive-confirm-dialog";
import type { useTaskArchiveConfirm } from "@/hooks/use-task-archive-confirm";

type TaskArchiveConfirmFlowProps = {
  taskId: string;
  archive: ReturnType<typeof useTaskArchiveConfirm>;
  /** Test id for the dialog's Archive button — the only per-surface difference. */
  confirmTestId: string;
};

/**
 * Binds a `useTaskArchiveConfirm` result to the shared confirmation dialog, so
 * every archive entry point maps the hook onto the dialog the same way and a
 * dialog prop change lands in one place.
 */
export function TaskArchiveConfirmFlow({
  taskId,
  archive,
  confirmTestId,
}: TaskArchiveConfirmFlowProps) {
  return (
    <TaskArchiveConfirmDialog
      open={archive.target !== null}
      onOpenChange={(open) => {
        if (!open) archive.closeConfirm();
      }}
      taskTitle={archive.target?.title ?? ""}
      taskId={taskId}
      executorType={archive.target?.executorType}
      isArchiving={archive.isPending}
      onConfirm={archive.confirmArchive}
      confirmTestId={confirmTestId}
    />
  );
}
