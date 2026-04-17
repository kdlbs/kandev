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

type ArchiveConfirmDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskTitle: string;
  isArchiving?: boolean;
  onConfirm: () => void;
};

export function ArchiveConfirmDialog({
  open,
  onOpenChange,
  taskTitle,
  isArchiving,
  onConfirm,
}: ArchiveConfirmDialogProps) {
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent onClick={(e) => e.stopPropagation()}>
        <AlertDialogHeader>
          <AlertDialogTitle>Archive task?</AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div>
              <p>Are you sure you want to archive &quot;{taskTitle}&quot;?</p>
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
            onClick={() => {
              if (isArchiving) return;
              onConfirm();
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
