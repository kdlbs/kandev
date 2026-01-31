'use client';

import { memo } from 'react';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@kandev/ui/alert-dialog';

type TaskPlanDialogsProps = {
  showDiscardDialog: boolean;
  showClearDialog: boolean;
  onDiscardDialogChange: (open: boolean) => void;
  onClearDialogChange: (open: boolean) => void;
  onDiscard: () => void;
  onClear: () => void;
};

export const TaskPlanDialogs = memo(function TaskPlanDialogs({
  showDiscardDialog,
  showClearDialog,
  onDiscardDialogChange,
  onClearDialogChange,
  onDiscard,
  onClear,
}: TaskPlanDialogsProps) {
  return (
    <>
      {/* Discard confirmation dialog */}
      <AlertDialog open={showDiscardDialog} onOpenChange={onDiscardDialogChange}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Discard changes?</AlertDialogTitle>
            <AlertDialogDescription>
              You have unsaved changes. Are you sure you want to discard them?
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel className="cursor-pointer">Cancel</AlertDialogCancel>
            <AlertDialogAction className="cursor-pointer" onClick={onDiscard}>
              Discard
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Clear plan confirmation dialog */}
      <AlertDialog open={showClearDialog} onOpenChange={onClearDialogChange}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Clear plan?</AlertDialogTitle>
            <AlertDialogDescription>
              This will permanently delete the plan. This action cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel className="cursor-pointer">Cancel</AlertDialogCancel>
            <AlertDialogAction
              className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={onClear}
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
});

