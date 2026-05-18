"use client";

import { useState } from "react";
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
import { Checkbox } from "@kandev/ui/checkbox";

export type TaskResetEnvConfirmDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  hasWorktreePath: boolean;
  isResetting?: boolean;
  onConfirm: (opts: { pushBranch: boolean }) => void;
};

export function TaskResetEnvConfirmDialog({
  open,
  onOpenChange,
  hasWorktreePath,
  isResetting,
  onConfirm,
}: TaskResetEnvConfirmDialogProps) {
  const [acknowledged, setAcknowledged] = useState(false);
  const [pushBranch, setPushBranch] = useState(false);

  const handleOpenChange = (next: boolean) => {
    if (!next) {
      // reset local state when closing
      setAcknowledged(false);
      setPushBranch(false);
    }
    onOpenChange(next);
  };

  return (
    <AlertDialog open={open} onOpenChange={handleOpenChange}>
      <AlertDialogContent onClick={(e) => e.stopPropagation()}>
        <AlertDialogHeader>
          <AlertDialogTitle>Reset environment?</AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div className="space-y-2">
              <p>
                This tears down the current container, sandbox, and/or worktree for this task so the
                next session starts with a fresh environment.
              </p>
              <p className="text-destructive">
                Any uncommitted or unpushed changes in the workspace will be lost.
              </p>
            </div>
          </AlertDialogDescription>
        </AlertDialogHeader>

        <div className="space-y-3 py-1">
          {hasWorktreePath && (
            <label className="flex items-start gap-2 text-sm cursor-pointer">
              <Checkbox
                checked={pushBranch}
                onCheckedChange={(v) => setPushBranch(v === true)}
                disabled={isResetting}
              />
              <span>
                Push the current branch to its remote before resetting.
                <span className="block text-xs text-muted-foreground">
                  Helps preserve committed work. Uncommitted changes cannot be pushed.
                </span>
              </span>
            </label>
          )}

          <label className="flex items-start gap-2 text-sm cursor-pointer">
            <Checkbox
              checked={acknowledged}
              onCheckedChange={(v) => setAcknowledged(v === true)}
              disabled={isResetting}
            />
            <span>I understand any uncommitted changes will be lost.</span>
          </label>
        </div>

        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">Cancel</AlertDialogCancel>
          <AlertDialogAction
            disabled={isResetting || !acknowledged}
            className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
            data-testid="reset-env-confirm"
            onClick={(e) => {
              // Radix's AlertDialogAction auto-closes the dialog on click,
              // which would dismiss it before the async reset completes — the
              // user never sees the loading spinner, and the dialog closes
              // even on failure. Block the auto-close so the parent owns the
              // close decision via `onOpenChange` after the promise settles.
              e.preventDefault();
              if (isResetting || !acknowledged) return;
              onConfirm({ pushBranch });
            }}
          >
            {isResetting ? <IconLoader className="mr-2 h-4 w-4 animate-spin" /> : null}
            Reset environment
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
