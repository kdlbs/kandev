"use client";

import { useEffect, useRef, useState } from "react";
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
import { useAppStore } from "@/components/state-provider";
import { useSubtaskCount } from "@/hooks/use-subtask-count";
import { getCleanupSummary, getBulkCleanupSummary } from "./task-cleanup-summary";

type TaskArchiveConfirmDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskTitle?: string;
  isBulkOperation?: boolean;
  count?: number;
  isArchiving?: boolean;
  taskId?: string;
  taskIds?: string[];
  /** Executor type of the task being archived (single). */
  executorType?: string | null;
  /** Executor types of the tasks being archived (bulk). */
  executorTypes?: Array<string | null | undefined>;
  onConfirm: (opts: { cascade: boolean }) => void;
  confirmTestId?: string;
};

type ArchiveOpenMode = "pending" | "confirm" | "bypass";

function useArchiveConfirmationMode(
  open: boolean,
  confirmTaskArchive: boolean,
  onConfirm: TaskArchiveConfirmDialogProps["onConfirm"],
  onOpenChange: TaskArchiveConfirmDialogProps["onOpenChange"],
) {
  const wasOpenRef = useRef(false);
  const [archiveOpenMode, setArchiveOpenMode] = useState<ArchiveOpenMode>("pending");

  useEffect(() => {
    const openedNow = open && !wasOpenRef.current;
    wasOpenRef.current = open;

    if (!open) {
      setArchiveOpenMode("pending");
      return;
    }
    if (!openedNow) return;

    if (confirmTaskArchive) {
      setArchiveOpenMode("confirm");
      return;
    }

    setArchiveOpenMode("bypass");
    onConfirm({ cascade: false });
    onOpenChange(false);
  }, [confirmTaskArchive, onConfirm, onOpenChange, open]);

  return archiveOpenMode === "confirm" || (archiveOpenMode === "pending" && confirmTaskArchive);
}

export function TaskArchiveConfirmDialog({
  open,
  onOpenChange,
  taskTitle,
  isBulkOperation,
  count,
  isArchiving,
  taskId,
  taskIds,
  executorType,
  executorTypes,
  onConfirm,
  confirmTestId,
}: TaskArchiveConfirmDialogProps) {
  const confirmTaskArchive = useAppStore((state) => state.userSettings?.confirmTaskArchive ?? true);
  const safeCount = count ?? 0;
  const label = isBulkOperation ? `task${safeCount !== 1 ? "s" : ""}` : "task";
  const title = isBulkOperation ? `Archive ${safeCount} ${label}?` : "Archive task?";
  const firstLine = isBulkOperation
    ? `Are you sure you want to archive ${safeCount} ${label}?`
    : `Are you sure you want to archive "${taskTitle}"?`;
  const cleanup = isBulkOperation
    ? getBulkCleanupSummary(executorTypes ?? [])
    : getCleanupSummary(executorType);

  const [cascade, setCascade] = useState(false);
  const requiresConfirmation = useArchiveConfirmationMode(
    open,
    confirmTaskArchive,
    onConfirm,
    onOpenChange,
  );
  const subtaskCount = useSubtaskCount(open && requiresConfirmation, taskId, taskIds);

  const handleOpenChange = (next: boolean) => {
    if (!next) setCascade(false);
    onOpenChange(next);
  };

  if (!requiresConfirmation) return null;

  return (
    <AlertDialog open={open} onOpenChange={handleOpenChange}>
      <AlertDialogContent onClick={(e) => e.stopPropagation()}>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div>
              <p>{firstLine}</p>
              {cleanup.lines.map((line, i) => (
                <p key={i} className="mt-2" data-testid="cleanup-line">
                  {line}
                </p>
              ))}
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
