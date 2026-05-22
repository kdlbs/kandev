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

type TaskDeleteConfirmDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskTitle?: string;
  isBulkOperation?: boolean;
  count?: number;
  isDeleting?: boolean;
  taskId?: string;
  taskIds?: string[];
  onConfirm: (opts: { cascade: boolean }) => void;
  confirmTestId?: string;
};

export function TaskDeleteConfirmDialog({
  open,
  onOpenChange,
  taskTitle,
  isBulkOperation,
  count,
  isDeleting,
  taskId,
  taskIds,
  onConfirm,
  confirmTestId,
}: TaskDeleteConfirmDialogProps) {
  const safeCount = count ?? 0;
  const label = isBulkOperation ? `task${safeCount !== 1 ? "s" : ""}` : "task";
  const title = isBulkOperation ? `Delete ${safeCount} ${label}` : "Delete task";
  const description = isBulkOperation
    ? `Are you sure you want to delete ${safeCount} ${label}? This action cannot be undone.`
    : `Are you sure you want to delete "${taskTitle}"? This action cannot be undone.`;

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
          <AlertDialogDescription>{description}</AlertDialogDescription>
        </AlertDialogHeader>
        {subtaskCount > 0 && (
          <label className="flex items-start gap-2 text-sm cursor-pointer">
            <Checkbox
              checked={cascade}
              onCheckedChange={(v) => setCascade(v === true)}
              disabled={isDeleting}
              data-testid="delete-cascade-checkbox"
            />
            <span>
              Also delete {subtaskCount} subtask{subtaskCount === 1 ? "" : "s"}
              <span className="block text-xs text-muted-foreground">
                Subtasks become root tasks unless you tick this. They may still be in progress.
              </span>
            </span>
          </label>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">Cancel</AlertDialogCancel>
          <AlertDialogAction
            disabled={isDeleting}
            className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
            data-testid={confirmTestId}
            onClick={() => {
              if (isDeleting) return;
              onConfirm({ cascade });
              handleOpenChange(false);
            }}
          >
            {isDeleting ? <IconLoader className="mr-2 h-4 w-4 animate-spin" /> : null}
            Delete
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

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
