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

const MAX_VISIBLE_FILES = 20;

export type DiscardLocalChangesDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  dirtyFiles: string[];
  repoPath?: string;
  onConfirm: () => void;
  onCancel: () => void;
};

export function DiscardLocalChangesDialog({
  open,
  onOpenChange,
  dirtyFiles,
  repoPath,
  onConfirm,
  onCancel,
}: DiscardLocalChangesDialogProps) {
  const visible = dirtyFiles.slice(0, MAX_VISIBLE_FILES);
  const overflow = dirtyFiles.length - visible.length;
  const target = repoPath ? ` in your local clone at ${repoPath}` : " in your local clone";

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent
        data-testid="discard-local-changes-dialog"
        onClick={(e) => e.stopPropagation()}
      >
        <AlertDialogHeader>
          <AlertDialogTitle>Discard local changes?</AlertDialogTitle>
          <AlertDialogDescription>
            Starting this task will permanently discard the uncommitted changes
            {target}. Back up anything you want to keep before continuing.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <ul
          className="max-h-48 overflow-auto rounded-md border border-border bg-muted/40 p-2 text-xs font-mono text-muted-foreground space-y-0.5"
          data-testid="discard-local-changes-files"
        >
          {visible.map((path) => (
            <li key={path} className="truncate" title={path}>
              {path}
            </li>
          ))}
          {overflow > 0 && (
            <li
              className="pt-1 text-[11px] italic text-muted-foreground/80"
              data-testid="discard-local-changes-overflow"
            >
              +{overflow} more
            </li>
          )}
        </ul>
        <AlertDialogFooter>
          <AlertDialogCancel
            className="cursor-pointer"
            data-testid="discard-local-changes-cancel"
            onClick={onCancel}
          >
            Cancel
          </AlertDialogCancel>
          <AlertDialogAction
            className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
            data-testid="discard-local-changes-confirm"
            onClick={onConfirm}
          >
            Discard and continue
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
