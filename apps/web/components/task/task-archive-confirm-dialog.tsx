"use client";

import { useEffect, useState } from "react";
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
import { getSubtaskCount } from "@/lib/api";

type TaskArchiveConfirmDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskTitle?: string;
  isBulkOperation?: boolean;
  count?: number;
  isArchiving?: boolean;
  taskId?: string;
  taskIds?: string[];
  onConfirm: (opts: { cascade: boolean }) => void;
  confirmTestId?: string;
};

export function TaskArchiveConfirmDialog({
  open,
  onOpenChange,
  taskTitle,
  isBulkOperation,
  count,
  isArchiving,
  taskId,
  taskIds,
  onConfirm,
  confirmTestId,
}: TaskArchiveConfirmDialogProps) {
  const safeCount = count ?? 0;
  const label = isBulkOperation ? `task${safeCount !== 1 ? "s" : ""}` : "task";
  const title = isBulkOperation ? `Archive ${safeCount} ${label}?` : "Archive task?";
  const firstLine = isBulkOperation
    ? `Are you sure you want to archive ${safeCount} ${label}?`
    : `Are you sure you want to archive "${taskTitle}"?`;

  const [cascade, setCascade] = useState(false);
  const subtaskCount = useSubtaskCount(open, taskId, taskIds);

  const handleOpenChange = (next: boolean) => {
    if (!next) setCascade(false);
    onOpenChange(next);
  };

  return (
    <AlertDialog open={open} onOpenChange={handleOpenChange}>
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
        {subtaskCount > 0 && (
          <label className="flex items-start gap-2 text-sm cursor-pointer">
            <Checkbox
              checked={cascade}
              onCheckedChange={(v) => setCascade(v === true)}
              disabled={isArchiving}
              data-testid="archive-cascade-checkbox"
            />
            <span>
              Also archive {subtaskCount} subtask{subtaskCount === 1 ? "" : "s"}
              <span className="block text-xs text-muted-foreground">
                Subtasks stay active unless you tick this. They may still be in progress.
              </span>
            </span>
          </label>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">Cancel</AlertDialogCancel>
          <AlertDialogAction
            disabled={isArchiving}
            className="cursor-pointer"
            data-testid={confirmTestId}
            onClick={() => {
              if (isArchiving) return;
              onConfirm({ cascade });
              handleOpenChange(false);
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

// useSubtaskCount fetches the subtask count for the dialog when it
// opens. Returns 0 while loading, when the request fails, or when no
// task identifier is supplied — in all of those cases the checkbox
// stays hidden, matching the pre-cascade-toggle behavior.
function useSubtaskCount(open: boolean, taskId?: string, taskIds?: string[]): number {
  const [total, setTotal] = useState(0);
  useEffect(() => {
    if (!open) return;
    const ids = taskIds ?? (taskId ? [taskId] : []);
    if (ids.length === 0) return;
    let cancelled = false;
    Promise.all(ids.map((id) => getSubtaskCount(id).catch(() => ({ count: 0 }))))
      .then((results) => {
        if (cancelled) return;
        setTotal(results.reduce((sum, r) => sum + r.count, 0));
      })
      .catch(() => {
        // swallow — leaves total at its prior value (0 on first run)
      });
    return () => {
      cancelled = true;
    };
  }, [open, taskId, taskIds]);
  return open ? total : 0;
}
