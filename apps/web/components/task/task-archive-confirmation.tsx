"use client";

import type { RefObject } from "react";
import { useTranslation } from "react-i18next";
import { IconLoader } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useSubtaskCountState, type SubtaskCountResult } from "@/hooks/use-subtask-count";
import { useTaskInFlight } from "@/hooks/use-task-in-flight";
import { getCleanupSummary } from "./task-cleanup-summary";
import { StillWorkingWarning } from "./task-still-working-warning";
import { TaskArchiveConfirmDialog } from "./task-archive-confirm-dialog";

type ArchiveConfirmValues = { cascade: boolean };

const ARCHIVE_LABEL_KEY = "task:archive";
const ARCHIVE_INLINE_CONFIRMATION_TEST_ID = "task-archive-inline-confirmation";
const DEFAULT_CONFIRM_TEST_ID = "archive-task-confirm";

export type TaskArchiveConfirmationProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskTitle?: string;
  taskId?: string;
  taskIds?: string[];
  isBulkOperation?: boolean;
  count?: number;
  isArchiving?: boolean;
  isInFlight?: boolean;
  executorType?: string | null;
  executorTypes?: Array<string | null | undefined>;
  anchorRef: RefObject<HTMLElement | null>;
  focusReturnRef?: RefObject<HTMLElement | null>;
  focusBoundaryRef?: RefObject<HTMLElement | null>;
  onConfirm: (values: ArchiveConfirmValues) => void | Promise<void>;
  confirmTestId?: string;
  /** Render a simple confirmation inside an existing action surface. */
  inline?: boolean;
};

function ArchiveDescription({
  taskTitle,
  cleanupLines,
  taskIsInFlight,
}: {
  taskTitle?: string;
  cleanupLines: string[];
  taskIsInFlight: boolean;
}) {
  const { t } = useTranslation();
  return (
    <span className="block space-y-2">
      <span className="block">{t("task:archiveTaskConfirm", { taskTitle })}</span>
      {cleanupLines.map((line) => (
        <span key={line} className="block">
          {line}
        </span>
      ))}
      {taskIsInFlight && <StillWorkingWarning />}
    </span>
  );
}

function ArchiveConfirmCopy({
  taskTitle,
  executorType,
  isInFlight,
  isArchiving,
  onCancel,
  onClose,
  onConfirm,
  confirmTestId,
}: {
  taskTitle?: string;
  executorType?: string | null;
  isInFlight: boolean;
  isArchiving?: boolean;
  onCancel: () => void;
  onClose?: () => void;
  onConfirm: () => void | Promise<void>;
  confirmTestId: string;
}) {
  const { t } = useTranslation();
  const description = (
    <ArchiveDescription
      taskTitle={taskTitle}
      cleanupLines={getCleanupSummary(executorType).lines}
      taskIsInFlight={isInFlight}
    />
  );
  return (
    <InlineConfirmActions
      density="touch"
      disabled={isArchiving}
      testId={ARCHIVE_INLINE_CONFIRMATION_TEST_ID}
      ariaLabel={t("task:archiveTaskTitle")}
      description={description}
      cancelLabel={t("common:cancel")}
      confirmLabel={t(ARCHIVE_LABEL_KEY)}
      confirmAriaLabel={t(ARCHIVE_LABEL_KEY)}
      confirmTestId={confirmTestId}
      onCancel={onCancel}
      onClose={onClose}
      onConfirm={onConfirm}
    />
  );
}

function ArchiveConfirmPopover({
  open,
  anchorRef,
  focusReturnRef,
  focusBoundaryRef,
  taskTitle,
  executorType,
  isInFlight,
  isArchiving,
  onOpenChange,
  onConfirm,
  confirmTestId,
}: {
  open: boolean;
  anchorRef: RefObject<HTMLElement | null>;
  focusReturnRef?: RefObject<HTMLElement | null>;
  focusBoundaryRef?: RefObject<HTMLElement | null>;
  taskTitle?: string;
  executorType?: string | null;
  isInFlight: boolean;
  isArchiving?: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void | Promise<void>;
  confirmTestId: string;
}) {
  const { t } = useTranslation();
  return (
    <ActionConfirmPopover
      open={open}
      size="wide"
      disabled={isArchiving}
      anchorRef={anchorRef}
      focusReturnRef={focusReturnRef}
      focusBoundaryRef={focusBoundaryRef ?? anchorRef}
      title={t("task:archiveTaskTitle")}
      description={
        <ArchiveDescription
          taskTitle={taskTitle}
          cleanupLines={getCleanupSummary(executorType).lines}
          taskIsInFlight={isInFlight}
        />
      }
      cancelLabel={t("common:cancel")}
      confirmLabel={t(ARCHIVE_LABEL_KEY)}
      confirmAriaLabel={t(ARCHIVE_LABEL_KEY)}
      confirmTestId={confirmTestId}
      testId="task-archive-confirm-popover"
      onOpenChange={onOpenChange}
      onCancel={() => onOpenChange(false)}
      onConfirm={onConfirm}
    />
  );
}

function ArchiveClassifyingPopover({
  anchorRef,
  focusReturnRef,
  focusBoundaryRef,
  onOpenChange,
  confirmTestId,
}: {
  anchorRef: RefObject<HTMLElement | null>;
  focusReturnRef?: RefObject<HTMLElement | null>;
  focusBoundaryRef?: RefObject<HTMLElement | null>;
  onOpenChange: (open: boolean) => void;
  confirmTestId: string;
}) {
  const { t } = useTranslation();
  return (
    <ActionConfirmPopover
      open
      size="wide"
      anchorRef={anchorRef}
      focusReturnRef={focusReturnRef}
      focusBoundaryRef={focusBoundaryRef ?? anchorRef}
      title={t("task:archiveTaskTitle")}
      description={
        <span className="flex items-center gap-2">
          <IconLoader className="h-4 w-4 animate-spin" />
          {t("common:loading")}
        </span>
      }
      cancelLabel={t("common:cancel")}
      confirmLabel={t(ARCHIVE_LABEL_KEY)}
      confirmAriaLabel={t(ARCHIVE_LABEL_KEY)}
      confirmTestId={confirmTestId}
      confirmDisabled
      testId="task-archive-confirm-popover"
      onOpenChange={onOpenChange}
      onCancel={() => onOpenChange(false)}
      onConfirm={() => undefined}
    />
  );
}

type ArchiveDialogProps = Pick<
  TaskArchiveConfirmationProps,
  | "onOpenChange"
  | "taskTitle"
  | "isBulkOperation"
  | "count"
  | "isArchiving"
  | "taskId"
  | "taskIds"
  | "isInFlight"
  | "executorType"
  | "executorTypes"
  | "onConfirm"
  | "confirmTestId"
> & {
  subtaskClassification: SubtaskCountResult;
};

function ArchiveDialog({ subtaskClassification, ...props }: ArchiveDialogProps) {
  return <TaskArchiveConfirmDialog open {...props} subtaskClassification={subtaskClassification} />;
}

type ArchiveConfirmationContentProps = ArchiveDialogProps & {
  confirmTaskArchive: boolean;
  isFinePointer: boolean;
  taskIsInFlight: boolean;
  anchorRef: RefObject<HTMLElement | null>;
  focusReturnRef?: RefObject<HTMLElement | null>;
  focusBoundaryRef?: RefObject<HTMLElement | null>;
  inline: boolean;
};

function ArchiveConfirmationContent({
  confirmTaskArchive,
  isFinePointer,
  taskIsInFlight,
  anchorRef,
  focusReturnRef,
  focusBoundaryRef,
  inline,
  subtaskClassification,
  ...dialogProps
}: ArchiveConfirmationContentProps) {
  const { t } = useTranslation();
  const shouldUseDialog =
    !confirmTaskArchive ||
    dialogProps.isBulkOperation ||
    subtaskClassification.status === "error" ||
    subtaskClassification.total > 0;

  if (shouldUseDialog) {
    return <ArchiveDialog {...dialogProps} subtaskClassification={subtaskClassification} />;
  }

  if (subtaskClassification.status !== "resolved") {
    if (inline) {
      return (
        <span data-testid="task-archive-classifying" className="text-xs text-muted-foreground">
          {t("common:loading")}
        </span>
      );
    }
    if (!isFinePointer) return null;
    return (
      <ArchiveClassifyingPopover
        anchorRef={anchorRef}
        focusReturnRef={focusReturnRef}
        focusBoundaryRef={focusBoundaryRef}
        onOpenChange={dialogProps.onOpenChange}
        confirmTestId={dialogProps.confirmTestId ?? DEFAULT_CONFIRM_TEST_ID}
      />
    );
  }

  const confirm = () => dialogProps.onConfirm({ cascade: false });
  if (inline || !isFinePointer) {
    return (
      <ArchiveConfirmCopy
        taskTitle={dialogProps.taskTitle}
        executorType={dialogProps.executorType}
        isInFlight={taskIsInFlight}
        isArchiving={dialogProps.isArchiving}
        onCancel={() => dialogProps.onOpenChange(false)}
        onClose={() => dialogProps.onOpenChange(false)}
        onConfirm={confirm}
        confirmTestId={dialogProps.confirmTestId ?? DEFAULT_CONFIRM_TEST_ID}
      />
    );
  }

  return (
    <ArchiveConfirmPopover
      open
      anchorRef={anchorRef}
      focusReturnRef={focusReturnRef}
      focusBoundaryRef={focusBoundaryRef}
      taskTitle={dialogProps.taskTitle}
      executorType={dialogProps.executorType}
      isInFlight={taskIsInFlight}
      isArchiving={dialogProps.isArchiving}
      onOpenChange={dialogProps.onOpenChange}
      onConfirm={confirm}
      confirmTestId={dialogProps.confirmTestId ?? DEFAULT_CONFIRM_TEST_ID}
    />
  );
}

export function TaskArchiveConfirmation({
  open,
  onOpenChange,
  taskTitle,
  taskId,
  taskIds,
  isBulkOperation = false,
  count,
  isArchiving,
  isInFlight,
  executorType,
  executorTypes,
  anchorRef,
  focusReturnRef,
  focusBoundaryRef,
  onConfirm,
  confirmTestId = DEFAULT_CONFIRM_TEST_ID,
  inline = false,
}: TaskArchiveConfirmationProps) {
  const { isFinePointer } = useResponsiveBreakpoint();
  const confirmTaskArchive = useAppStore((state) => state.userSettings?.confirmTaskArchive ?? true);
  const classification = useSubtaskCountState(open && confirmTaskArchive, taskId, taskIds);
  const storeInFlight = useTaskInFlight(taskId, taskIds, open && confirmTaskArchive);
  const taskIsInFlight = Boolean(isInFlight) || storeInFlight;

  if (!open) return null;

  return (
    <ArchiveConfirmationContent
      confirmTaskArchive={confirmTaskArchive}
      isFinePointer={isFinePointer}
      taskIsInFlight={taskIsInFlight}
      anchorRef={anchorRef}
      focusReturnRef={focusReturnRef}
      focusBoundaryRef={focusBoundaryRef}
      inline={inline}
      onOpenChange={onOpenChange}
      taskTitle={taskTitle}
      isBulkOperation={isBulkOperation}
      count={count}
      isArchiving={isArchiving}
      taskId={taskId}
      taskIds={taskIds}
      isInFlight={isInFlight}
      executorType={executorType}
      executorTypes={executorTypes}
      onConfirm={onConfirm}
      confirmTestId={confirmTestId}
      subtaskClassification={classification}
    />
  );
}
