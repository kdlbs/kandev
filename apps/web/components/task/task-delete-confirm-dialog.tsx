"use client";

import { useState, type RefObject } from "react";
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
import {
  getCleanupSummary,
  getBulkCleanupSummary,
  hasWorktreeExecutor,
} from "./task-cleanup-summary";
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
  /** Require discard consent when the task's retained worktree state is unknown. */
  requireDiscardConsent?: boolean;
  onConfirm: (opts: { cascade: boolean; discardWorktreeChanges: boolean }) => void;
  confirmTestId?: string;
  /** Element to return keyboard focus to on close, confirmed or cancelled
   * (AC-TASKS-TASK-ACTIONS-MENU-001.12). Omitted callers keep Radix's
   * default restore-to-previously-focused-element behavior. */
  focusReturnRef?: RefObject<HTMLElement | null>;
};

type DiscardWorktreeChangesOptionProps = {
  enabled: boolean;
  checked: boolean;
  disabled?: boolean;
  onCheckedChange: (checked: boolean) => void;
};

function DiscardWorktreeChangesOption({
  enabled,
  checked,
  disabled,
  onCheckedChange,
}: DiscardWorktreeChangesOptionProps) {
  const { t } = useTranslation();
  if (!enabled) return null;
  return (
    <label className="flex min-h-11 cursor-pointer items-start gap-2 text-sm">
      <Checkbox
        checked={checked}
        onCheckedChange={(value) => onCheckedChange(value === true)}
        disabled={disabled}
        data-testid="delete-discard-worktree-checkbox"
      />
      <span>
        {t("task:discardWorktreeChanges")}
        <span className="block text-sm text-muted-foreground">
          {t("task:discardWorktreeChangesDescription")}
        </span>
      </span>
    </label>
  );
}

type CascadeOptionProps = {
  count: number;
  checked: boolean;
  disabled?: boolean;
  onCheckedChange: (checked: boolean) => void;
};

function CascadeOption({ count, checked, disabled, onCheckedChange }: CascadeOptionProps) {
  const { t } = useTranslation();
  return (
    <label className="flex min-h-11 cursor-pointer items-start gap-2 text-sm">
      <Checkbox
        checked={checked}
        onCheckedChange={(value) => onCheckedChange(value === true)}
        disabled={disabled}
        data-testid="delete-cascade-checkbox"
      />
      <span>
        {t("task:alsoDeleteSubtasks", { count })}
        <span className="block text-sm text-muted-foreground">
          {t("task:subtasksBecomeRootTasksUnlessYou")}
        </span>
      </span>
    </label>
  );
}

type TaskDeleteActionProps = {
  isDeleting?: boolean;
  disabled: boolean;
  confirmTestId?: string;
  cascade: boolean;
  discardWorktreeChanges: boolean;
  onConfirm: (opts: { cascade: boolean; discardWorktreeChanges: boolean }) => void;
  onClose: () => void;
};

function TaskDeleteAction({
  isDeleting,
  disabled,
  confirmTestId,
  cascade,
  discardWorktreeChanges,
  onConfirm,
  onClose,
}: TaskDeleteActionProps) {
  const { t } = useTranslation();
  return (
    <AlertDialogAction
      variant="destructive"
      disabled={disabled}
      className={TASK_CONFIRM_ACTION_CLASS}
      data-testid={confirmTestId}
      onClick={() => {
        if (isDeleting) return;
        onConfirm({ cascade, discardWorktreeChanges });
        onClose();
      }}
    >
      {isDeleting ? <IconLoader className="mr-2 h-4 w-4 animate-spin" /> : null}
      {t("task:delete")}
    </AlertDialogAction>
  );
}

function useTaskDeleteDialogState(onOpenChange: (open: boolean) => void) {
  const [cascade, setCascade] = useState(false);
  const [discardWorktreeChanges, setDiscardWorktreeChanges] = useState(false);
  const handleOpenChange = (next: boolean) => {
    if (!next) {
      setCascade(false);
      setDiscardWorktreeChanges(false);
    }
    onOpenChange(next);
  };
  return {
    cascade,
    setCascade,
    discardWorktreeChanges,
    setDiscardWorktreeChanges,
    handleOpenChange,
  };
}

function hasPotentialWorktree(
  isBulkOperation: boolean | undefined,
  executorType: string | null | undefined,
  executorTypes: Array<string | null | undefined> | undefined,
) {
  if (isBulkOperation) {
    return (
      executorTypes == null ||
      executorTypes.some((type) => type == null || hasWorktreeExecutor(type))
    );
  }
  return executorType == null || hasWorktreeExecutor(executorType);
}

function shouldRequireDiscardConsent(
  explicit: boolean,
  hasWorktree: boolean,
  subtaskCount: number,
) {
  return explicit || hasWorktree || subtaskCount > 0;
}

function TaskDeleteDialogOptions({
  isInFlight,
  isBulkOperation,
  safeCount,
  storeInFlight,
  requiresDiscardConsent,
  discardWorktreeChanges,
  setDiscardWorktreeChanges,
  isDeleting,
  subtaskCount,
  cascade,
  setCascade,
}: {
  isInFlight?: boolean;
  isBulkOperation?: boolean;
  safeCount: number;
  storeInFlight: boolean;
  requiresDiscardConsent: boolean;
  discardWorktreeChanges: boolean;
  setDiscardWorktreeChanges: (checked: boolean) => void;
  isDeleting?: boolean;
  subtaskCount: number;
  cascade: boolean;
  setCascade: (checked: boolean) => void;
}) {
  return (
    <>
      {(isInFlight || storeInFlight) && (
        <StillWorkingWarning count={isBulkOperation ? safeCount : undefined} />
      )}
      <DiscardWorktreeChangesOption
        enabled={requiresDiscardConsent}
        checked={discardWorktreeChanges}
        onCheckedChange={setDiscardWorktreeChanges}
        disabled={isDeleting}
      />
      {subtaskCount > 0 && (
        <CascadeOption
          count={subtaskCount}
          checked={cascade}
          onCheckedChange={setCascade}
          disabled={isDeleting}
        />
      )}
    </>
  );
}

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
  requireDiscardConsent = false,
  onConfirm,
  confirmTestId,
  focusReturnRef,
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

  const {
    cascade,
    setCascade,
    discardWorktreeChanges,
    setDiscardWorktreeChanges,
    handleOpenChange,
  } = useTaskDeleteDialogState(onOpenChange);
  const subtaskCount = useSubtaskCount(open, taskId, taskIds);
  const storeInFlight = useTaskInFlight(taskId, taskIds, open);
  const requiresDiscardConsent = shouldRequireDiscardConsent(
    requireDiscardConsent,
    hasPotentialWorktree(isBulkOperation, executorType, executorTypes),
    subtaskCount,
  );

  return (
    <AlertDialog open={open} onOpenChange={handleOpenChange}>
      <AlertDialogContent
        size="lg"
        className={TASK_CONFIRM_CLASS}
        onClick={stopDialogPropagation}
        onCloseAutoFocus={(event) => {
          const target = focusReturnRef?.current;
          if (!target || !document.contains(target)) return;
          event.preventDefault();
          target.focus();
        }}
      >
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
          <TaskDeleteDialogOptions
            isInFlight={isInFlight}
            isBulkOperation={isBulkOperation}
            safeCount={safeCount}
            storeInFlight={storeInFlight}
            requiresDiscardConsent={requiresDiscardConsent}
            discardWorktreeChanges={discardWorktreeChanges}
            setDiscardWorktreeChanges={setDiscardWorktreeChanges}
            isDeleting={isDeleting}
            subtaskCount={subtaskCount}
            cascade={cascade}
            setCascade={setCascade}
          />
        </div>
        <AlertDialogFooter className={TASK_CONFIRM_FOOTER_CLASS}>
          <AlertDialogCancel className={TASK_CONFIRM_ACTION_CLASS}>
            {t("common:cancel")}
          </AlertDialogCancel>
          <TaskDeleteAction
            disabled={isDeleting || (requiresDiscardConsent && !discardWorktreeChanges)}
            isDeleting={isDeleting}
            confirmTestId={confirmTestId}
            cascade={cascade}
            discardWorktreeChanges={discardWorktreeChanges}
            onConfirm={onConfirm}
            onClose={() => handleOpenChange(false)}
          />
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
