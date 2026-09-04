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
import { useSubtaskCountState, type SubtaskCountResult } from "@/hooks/use-subtask-count";
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

type TaskArchiveConfirmDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskTitle?: string;
  isBulkOperation?: boolean;
  count?: number;
  isArchiving?: boolean;
  taskId?: string;
  taskIds?: string[];
  isInFlight?: boolean;
  /** Executor type of the task being archived (single). */
  executorType?: string | null;
  /** Executor types of the tasks being archived (bulk). */
  executorTypes?: Array<string | null | undefined>;
  onConfirm: (opts: { cascade: boolean }) => void;
  confirmTestId?: string;
  /** Preflight result supplied by the local confirmation adapter. */
  subtaskClassification?: SubtaskCountResult;
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

function shouldCheckTaskInFlight(open: boolean, requiresConfirmation: boolean): boolean {
  return open && requiresConfirmation;
}

function computeTaskIsInFlight(isInFlight: boolean | undefined, storeInFlight: boolean): boolean {
  return Boolean(isInFlight) || storeInFlight;
}

function isArchiveActionDisabled(
  isArchiving: boolean | undefined,
  classification: SubtaskCountResult,
): boolean {
  return (
    Boolean(isArchiving) || classification.status === "idle" || classification.status === "loading"
  );
}

// The legacy cascade dialog intentionally keeps its state, preference bypass,
// and cleanup copy in one boundary.
// eslint-disable-next-line max-lines-per-function
export function TaskArchiveConfirmDialog({
  open,
  onOpenChange,
  taskTitle,
  isBulkOperation,
  count,
  isArchiving,
  taskId,
  taskIds,
  isInFlight,
  executorType,
  executorTypes,
  onConfirm,
  confirmTestId,
  subtaskClassification,
}: TaskArchiveConfirmDialogProps) {
  const { t } = useTranslation();
  const confirmTaskArchive = useAppStore((state) => state.userSettings?.confirmTaskArchive ?? true);
  const safeCount = count ?? 0;
  const title = isBulkOperation
    ? t("task:archiveTasksTitle", { count: safeCount })
    : t("task:archiveTaskTitle");
  const firstLine = isBulkOperation
    ? t("task:archiveTasksConfirm", { count: safeCount })
    : t("task:archiveTaskConfirm", { taskTitle });
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
  const fetchedSubtaskClassification = useSubtaskCountState(
    open && requiresConfirmation && !subtaskClassification,
    taskId,
    taskIds,
  );
  const classification = subtaskClassification ?? fetchedSubtaskClassification;
  const archiveDisabled = isArchiveActionDisabled(isArchiving, classification);
  const subtaskCount = classification.status === "resolved" ? classification.total : 0;
  const shouldCheckInFlight = shouldCheckTaskInFlight(open, requiresConfirmation);
  const storeInFlight = useTaskInFlight(taskId, taskIds, shouldCheckInFlight);
  const taskIsInFlight = computeTaskIsInFlight(isInFlight, storeInFlight);

  const handleOpenChange = (next: boolean) => {
    if (!next) setCascade(false);
    onOpenChange(next);
  };

  if (!requiresConfirmation) return null;

  return (
    <AlertDialog open={open} onOpenChange={handleOpenChange}>
      <AlertDialogContent size="lg" className={TASK_CONFIRM_CLASS} onClick={stopDialogPropagation}>
        <AlertDialogHeader className={TASK_CONFIRM_HEADER_CLASS}>
          <AlertDialogTitle className="text-base font-semibold">{title}</AlertDialogTitle>
        </AlertDialogHeader>
        <div data-testid="task-confirmation-body" className={TASK_CONFIRM_BODY_CLASS}>
          <AlertDialogDescription asChild className="text-left text-sm leading-6">
            <div className="space-y-3">
              <p data-testid="task-confirmation-outcome">{firstLine}</p>
              <TaskCleanupConsequences summary={cleanup} />
            </div>
          </AlertDialogDescription>
          {taskIsInFlight && (
            <StillWorkingWarning count={isBulkOperation ? safeCount : undefined} />
          )}
          {subtaskCount > 0 && (
            <label className="flex cursor-pointer items-start gap-2 text-sm">
              <Checkbox
                checked={cascade}
                onCheckedChange={(v) => setCascade(v === true)}
                disabled={isArchiving}
                data-testid="archive-cascade-checkbox"
              />
              <span>
                {t("task:alsoArchiveSubtasks", { count: subtaskCount })}
                <span className="block text-sm text-muted-foreground">
                  {t("task:subtasksStayActiveUnlessYouTick")}
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
            variant="default"
            disabled={archiveDisabled}
            className={TASK_CONFIRM_ACTION_CLASS}
            data-testid={confirmTestId}
            onClick={() => {
              if (archiveDisabled) return;
              onConfirm({ cascade });
              handleOpenChange(false);
            }}
          >
            {isArchiving ? <IconLoader className="mr-2 h-4 w-4 animate-spin" /> : null}
            {t("task:archive")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
