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

type TaskArchiveConfirmDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskTitle?: string;
  isBulkOperation?: boolean;
  count?: number;
  isArchiving?: boolean;
  onConfirm: () => void;
  confirmTestId?: string;
};

export function TaskArchiveConfirmDialog({
  open,
  onOpenChange,
  taskTitle,
  isBulkOperation,
  count,
  isArchiving,
  onConfirm,
  confirmTestId,
}: TaskArchiveConfirmDialogProps) {
  const safeCount = count ?? 0;
  const label = isBulkOperation ? `task${safeCount !== 1 ? "s" : ""}` : "task";
  const title = isBulkOperation ? `Archive ${safeCount} ${label}?` : "Archive task?";
  const firstLine = isBulkOperation
    ? `Are you sure you want to archive ${safeCount} ${label}?`
    : `Are you sure you want to archive "${taskTitle}"?`;

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent onClick={(e) => e.stopPropagation()}>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div>
              <p>{firstLine}</p>
              <p className="mt-2">
                This will delete the task&apos;s worktree and stop any running agent sessions.
              </p>
            </div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">Cancel</AlertDialogCancel>
          <AlertDialogAction
            disabled={isArchiving}
            className="cursor-pointer"
            data-testid={confirmTestId}
            onClick={() => {
              if (isArchiving) return;
              onConfirm();
              onOpenChange(false);
            }}
          >
            {isArchiving ? <IconLoader className="mr-2 h-4 w-4 animate-spin" /> : null}
            Archive
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
