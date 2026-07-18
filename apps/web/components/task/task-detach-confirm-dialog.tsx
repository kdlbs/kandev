"use client";

import { IconLoader } from "@tabler/icons-react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";

type TaskDetachConfirmDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskTitle?: string;
  sharesParentWorkspace?: boolean;
  isDetaching?: boolean;
  onConfirm: () => void;
};

export function TaskDetachConfirmDialog({
  open,
  onOpenChange,
  taskTitle,
  sharesParentWorkspace,
  isDetaching,
  onConfirm,
}: TaskDetachConfirmDialogProps) {
  return (
    <AlertDialog
      open={open}
      onOpenChange={(next) => {
        if (!isDetaching) onOpenChange(next);
      }}
    >
      <AlertDialogContent onClick={(event) => event.stopPropagation()}>
        <AlertDialogHeader>
          <AlertDialogTitle>Detach task from parent?</AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div className="space-y-2">
              <p>
                &quot;{taskTitle || "This task"}&quot; will become a top-level task. Its workflow,
                subtasks, and state will not change.
              </p>
              <p>
                Detaching changes the hierarchy only. Access to any shared workspace remains
                unchanged.
              </p>
              {sharesParentWorkspace && (
                <p className="font-medium text-foreground">
                  This task shares its parent&apos;s workspace. Current and future sessions will
                  keep using that shared workspace.
                </p>
              )}
            </div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={isDetaching} className="cursor-pointer">
            Cancel
          </AlertDialogCancel>
          <AlertDialogAction
            disabled={isDetaching}
            className="cursor-pointer"
            data-testid="detach-task-confirm"
            onClick={(event) => {
              event.preventDefault();
              if (!isDetaching) onConfirm();
            }}
          >
            {isDetaching && <IconLoader className="mr-2 h-4 w-4 animate-spin" />}
            Detach
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

export function TaskDetachTargetConfirmDialog({
  target,
  detachingTaskId,
  onDismiss,
  onConfirm,
}: {
  target: {
    id: string;
    title: string;
    workspaceMode?: "inherit_parent" | "new_workspace" | "shared_group";
  } | null;
  detachingTaskId: string | null;
  onDismiss: () => void;
  onConfirm: () => void;
}) {
  return (
    <TaskDetachConfirmDialog
      open={target !== null}
      onOpenChange={(open) => {
        if (!open) onDismiss();
      }}
      taskTitle={target?.title}
      sharesParentWorkspace={target?.workspaceMode === "inherit_parent"}
      isDetaching={target?.id === detachingTaskId}
      onConfirm={onConfirm}
    />
  );
}
