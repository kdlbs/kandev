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

type QuickChatDeleteDialogProps = {
  sessionToDelete: string | null;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
};

export function QuickChatDeleteDialog({
  sessionToDelete,
  onOpenChange,
  onConfirm,
}: QuickChatDeleteDialogProps) {
  return (
    <AlertDialog open={!!sessionToDelete} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete Quick Chat?</AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div>
              <p>This will permanently delete this quick chat session, including:</p>
              <ul className="list-disc list-inside mt-2 space-y-1">
                <li>All conversation history</li>
                <li>The task and its data</li>
                <li>The associated worktree</li>
              </ul>
              <p className="mt-2">This action cannot be undone.</p>
            </div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">Cancel</AlertDialogCancel>
          <AlertDialogAction
            onClick={onConfirm}
            className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            Delete
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

