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
import { useSubtaskCount } from "@/hooks/use-subtask-count";
import { useTaskInFlight } from "@/hooks/use-task-in-flight";
import { getCleanupSummary, getBulkCleanupSummary } from "./task-cleanup-summary";
import { StillWorkingWarning } from "./task-still-working-warning";
import { useTranslation } from "react-i18next";

type TaskDeleteConfirmDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskTitle?: string;
  isBulkOperation?: boolean;
  count?: number;
  isDeleting?: boolean;
  taskId?: string;
  taskIds?: string[];
  isInFlight?: boolean;
  /** Executor type of the task being deleted (single). */
  executorType?: string | null;
  /** Executor types of the tasks being deleted (bulk). */
  executorTypes?: Array<string | null | undefined>;
  onConfirm: (opts: { cascade: boolean }) => void;
  confirmTestId?: string;
};

const dialogClass =
  "w-[calc(100vw-2rem)] !max-w-[calc(100vw-2rem)] font-sans !text-sm sm:!max-w-lg";

export function TaskDeleteConfirmDialog({
  open,
  onOpenChange,
  taskTitle,
  isBulkOperation,
  count,
  isDeleting,
  taskId,
  taskIds,
  isInFlight,
  executorType,
  executorTypes,
  onConfirm,
  confirmTestId,
}: TaskDeleteConfirmDialogProps) {
  const { t } = useTranslation();
  const safeCount = count ?? 0;
  const title = isBulkOperation
    ? t("task:deleteTasksTitle", { count: safeCount })
    : t("task:deleteTaskTitle");
  const description = isBulkOperation
    ? t("task:deleteTasksConfirm", { count: safeCount })
    : t("task:deleteTaskConfirm", { taskTitle });
  const cleanup = isBulkOperation
    ? getBulkCleanupSummary(executorTypes ?? [])
    : getCleanupSummary(executorType);

  const [cascade, setCascade] = useState(false);
  const subtaskCount = useSubtaskCount(open, taskId, taskIds);
  const storeInFlight = useTaskInFlight(taskId, taskIds, open);

  const handleOpenChange = (next: boolean) => {
    if (!next) setCascade(false);
    onOpenChange(next);
  };

  return (
    <AlertDialog open={open} onOpenChange={handleOpenChange}>
      <AlertDialogContent className={dialogClass} onClick={(e) => e.stopPropagation()}>
        <AlertDialogHeader>
          <AlertDialogTitle className="text-base font-semibold">{title}</AlertDialogTitle>
          <AlertDialogDescription asChild className="text-sm leading-6">
            <div>
              <p>{description}</p>
              {cleanup.lines.map((line, i) => (
                <p key={i} className="mt-2" data-testid="cleanup-line">
                  {line}
                </p>
              ))}
            </div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        {(isInFlight || storeInFlight) && (
          <StillWorkingWarning count={isBulkOperation ? safeCount : undefined} />
        )}
        {subtaskCount > 0 && (
          <label className="flex items-start gap-2 text-sm cursor-pointer">
            <Checkbox
              checked={cascade}
              onCheckedChange={(v) => setCascade(v === true)}
              disabled={isDeleting}
              data-testid="delete-cascade-checkbox"
            />
            <span>
              {t("task:alsoDeleteSubtasks", { count: subtaskCount })}
              <span className="block text-sm text-muted-foreground">
                {t("task:subtasksBecomeRootTasksUnlessYou")}
              </span>
            </span>
          </label>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer !text-sm">
            {t("common:cancel")}
          </AlertDialogCancel>
          <AlertDialogAction
            disabled={isDeleting}
            className="cursor-pointer bg-destructive !text-sm text-destructive-foreground hover:bg-destructive/90"
            data-testid={confirmTestId}
            onClick={() => {
              if (isDeleting) return;
              onConfirm({ cascade });
              handleOpenChange(false);
            }}
          >
            {isDeleting ? <IconLoader className="mr-2 h-4 w-4 animate-spin" /> : null}
            {t("task:delete")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
