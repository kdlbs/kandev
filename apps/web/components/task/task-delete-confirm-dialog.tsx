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
import { TaskCleanupConsequences } from "./task-cleanup-consequences";
import { StillWorkingWarning } from "./task-still-working-warning";
import {
  TASK_CONFIRM_ACTION_CLASS,
  TASK_CONFIRM_BODY_CLASS,
  TASK_CONFIRM_CLASS,
  TASK_CONFIRM_FOOTER_CLASS,
  TASK_CONFIRM_HEADER_CLASS,
  stopDialogPropagation,
} from "./task-confirm-dialog-shared";
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
      <AlertDialogContent size="lg" className={TASK_CONFIRM_CLASS} onClick={stopDialogPropagation}>
        <AlertDialogHeader className={TASK_CONFIRM_HEADER_CLASS}>
          <AlertDialogTitle className="text-base font-semibold">{title}</AlertDialogTitle>
        </AlertDialogHeader>
        <div data-testid="task-confirmation-body" className={TASK_CONFIRM_BODY_CLASS}>
          <AlertDialogDescription asChild className="text-left text-sm leading-6">
            <div className="space-y-3">
              <p data-testid="task-confirmation-outcome">{description}</p>
              <TaskCleanupConsequences summary={cleanup} />
            </div>
          </AlertDialogDescription>
          {(isInFlight || storeInFlight) && (
            <StillWorkingWarning count={isBulkOperation ? safeCount : undefined} />
          )}
          {subtaskCount > 0 && (
            <label className="flex cursor-pointer items-start gap-2 text-sm">
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
        </div>
        <AlertDialogFooter className={TASK_CONFIRM_FOOTER_CLASS}>
          <AlertDialogCancel className={TASK_CONFIRM_ACTION_CLASS}>
            {t("common:cancel")}
          </AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={isDeleting}
            className={TASK_CONFIRM_ACTION_CLASS}
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
