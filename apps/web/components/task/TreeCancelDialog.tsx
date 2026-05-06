"use client";

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

type TreeCancelDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskCount: number;
  activeRunCount: number;
  onConfirm: () => void;
};

export function TreeCancelDialog({
  open,
  onOpenChange,
  taskCount,
  activeRunCount,
  onConfirm,
}: TreeCancelDialogProps) {
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Cancel task tree</AlertDialogTitle>
          <AlertDialogDescription>
            This will cancel {taskCount} tasks and interrupt {activeRunCount} active agent sessions.
            This action can be undone with Restore.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">Keep running</AlertDialogCancel>
          <AlertDialogAction className="cursor-pointer" onClick={onConfirm}>
            Cancel tree
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
