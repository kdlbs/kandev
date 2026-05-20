"use client";

import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";

type DeleteRepositoryDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDelete: () => void;
  activeSessionCount: number;
  deleteLoading: boolean;
};

export function DeleteRepositoryDialog({
  open,
  onOpenChange,
  onDelete,
  activeSessionCount,
  deleteLoading,
}: DeleteRepositoryDialogProps) {
  const hasActiveSessions = activeSessionCount > 0;
  const sessionWord = activeSessionCount === 1 ? "session" : "sessions";
  const isOne = activeSessionCount === 1;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete repository</DialogTitle>
          <DialogDescription>
            {hasActiveSessions
              ? `This repository is used by ${activeSessionCount} active agent ${sessionWord}. Stop or finish ${isOne ? "it" : "them"} before deleting the repository.`
              : "This will remove the repository and its scripts. This action cannot be undone."}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            className="cursor-pointer"
            onClick={() => onOpenChange(false)}
          >
            {hasActiveSessions ? "Close" : "Cancel"}
          </Button>
          {!hasActiveSessions && (
            <Button
              type="button"
              variant="destructive"
              className="cursor-pointer"
              onClick={onDelete}
              disabled={deleteLoading}
            >
              Delete Repository
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
