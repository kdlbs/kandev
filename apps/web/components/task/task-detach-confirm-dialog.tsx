"use client";

import type { ReactNode, RefObject } from "react";
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
import { useTranslation } from "react-i18next";
import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";
import { MobileActionConfirmation } from "@/components/confirmation/mobile-action-confirmation";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

const DETACH_TASK_FROM_PARENT_KEY = "task:detachTaskFromParent";
const DETACH_LABEL_KEY = "task:detach";
const DETACH_CONFIRM_TEST_ID = "detach-task-confirm";
const CANCEL_KEY = "common:cancel";

export type TaskDetachConfirmationCopyProps = {
  taskTitle?: string;
  sharesParentWorkspace?: boolean;
};

function TaskDetachDescription({
  taskTitle,
  sharesParentWorkspace,
  structured = false,
}: TaskDetachConfirmationCopyProps & { structured?: boolean }): ReactNode {
  const { t } = useTranslation();
  const title = taskTitle || t("task:thisTask2");

  if (structured) {
    return (
      <>
        <p className="break-words">
          {t("task:detachWillBecomeTopLevel", {
            taskTitle: title,
          })}
        </p>
        <p>{t("task:detachingChangesTheHierarchyOnlyAccess")}</p>
        {sharesParentWorkspace && (
          <p className="font-medium text-foreground">{t("task:thisTaskSharesItsParentS")}</p>
        )}
      </>
    );
  }

  return (
    <span className="block space-y-2">
      <span className="block">
        {t("task:detachWillBecomeTopLevel", {
          taskTitle: title,
        })}
      </span>
      <span className="block">{t("task:detachingChangesTheHierarchyOnlyAccess")}</span>
      {sharesParentWorkspace && (
        <span className="block font-medium text-foreground">
          {t("task:thisTaskSharesItsParentS")}
        </span>
      )}
    </span>
  );
}

function detachConfirmAriaLabel(t: (key: string) => string, taskTitle?: string): string {
  return taskTitle ? `${t(DETACH_LABEL_KEY)} ${taskTitle}` : t(DETACH_TASK_FROM_PARENT_KEY);
}

export type TaskDetachConfirmPopoverProps = TaskDetachConfirmationCopyProps & {
  open: boolean;
  anchorRef: RefObject<HTMLElement | null>;
  focusReturnRef?: RefObject<HTMLElement | null>;
  focusBoundaryRef?: RefObject<HTMLElement | null>;
  onOpenChange: (open: boolean) => void;
  onCancel?: () => void;
  onConfirm: () => void | Promise<void>;
};

export function TaskDetachConfirmPopover({
  open,
  anchorRef,
  focusReturnRef,
  focusBoundaryRef,
  taskTitle,
  sharesParentWorkspace,
  onOpenChange,
  onCancel,
  onConfirm,
}: TaskDetachConfirmPopoverProps) {
  const { t } = useTranslation();
  return (
    <ActionConfirmPopover
      open={open}
      anchorRef={anchorRef}
      focusReturnRef={focusReturnRef}
      focusBoundaryRef={focusBoundaryRef}
      title={t(DETACH_TASK_FROM_PARENT_KEY)}
      description={
        <TaskDetachDescription
          taskTitle={taskTitle}
          sharesParentWorkspace={sharesParentWorkspace}
        />
      }
      cancelLabel={t(CANCEL_KEY)}
      confirmLabel={t(DETACH_LABEL_KEY)}
      confirmAriaLabel={detachConfirmAriaLabel(t, taskTitle)}
      confirmTestId={DETACH_CONFIRM_TEST_ID}
      testId="detach-task-confirm-popover"
      onOpenChange={onOpenChange}
      onCancel={onCancel}
      onConfirm={onConfirm}
    />
  );
}

export type TaskDetachInlineConfirmationProps = TaskDetachConfirmationCopyProps & {
  onCancel: () => void;
  onClose?: () => void;
  onConfirm: () => void | Promise<void>;
};

export function TaskDetachInlineConfirmation({
  taskTitle,
  sharesParentWorkspace,
  onCancel,
  onClose,
  onConfirm,
}: TaskDetachInlineConfirmationProps) {
  const { t } = useTranslation();
  return (
    <InlineConfirmActions
      density="touch"
      testId="detach-task-inline-confirmation"
      ariaLabel={t(DETACH_TASK_FROM_PARENT_KEY)}
      description={
        <TaskDetachDescription
          taskTitle={taskTitle}
          sharesParentWorkspace={sharesParentWorkspace}
        />
      }
      cancelLabel={t(CANCEL_KEY)}
      confirmLabel={t(DETACH_LABEL_KEY)}
      confirmAriaLabel={detachConfirmAriaLabel(t, taskTitle)}
      confirmTestId={DETACH_CONFIRM_TEST_ID}
      onCancel={onCancel}
      onClose={onClose}
      onConfirm={onConfirm}
    />
  );
}

export type TaskDetachConfirmationSurfaceProps = TaskDetachConfirmationCopyProps & {
  taskId: string;
  open: boolean;
  anchorRef: RefObject<HTMLElement | null>;
  focusReturnRef?: RefObject<HTMLElement | null>;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void | Promise<void>;
};

export function TaskDetachConfirmationSurface({
  taskId,
  open,
  anchorRef,
  focusReturnRef,
  taskTitle,
  sharesParentWorkspace,
  onOpenChange,
  onConfirm,
}: TaskDetachConfirmationSurfaceProps) {
  const { t } = useTranslation();
  const { isFinePointer } = useResponsiveBreakpoint();
  const fallback = isFinePointer ? (
    <TaskDetachConfirmPopover
      open={open}
      anchorRef={anchorRef}
      focusReturnRef={focusReturnRef}
      focusBoundaryRef={anchorRef}
      taskTitle={taskTitle}
      sharesParentWorkspace={sharesParentWorkspace}
      onOpenChange={onOpenChange}
      onCancel={() => onOpenChange(false)}
      onConfirm={onConfirm}
    />
  ) : (
    <TaskDetachInlineConfirmation
      taskTitle={taskTitle}
      sharesParentWorkspace={sharesParentWorkspace}
      onCancel={() => onOpenChange(false)}
      onClose={() => onOpenChange(false)}
      onConfirm={onConfirm}
    />
  );
  return (
    <MobileActionConfirmation
      open={open}
      targetKey={taskId}
      title={t(DETACH_TASK_FROM_PARENT_KEY)}
      description={
        <TaskDetachDescription
          taskTitle={taskTitle}
          sharesParentWorkspace={sharesParentWorkspace}
        />
      }
      cancelLabel={t(CANCEL_KEY)}
      confirmLabel={t(DETACH_LABEL_KEY)}
      confirmAriaLabel={detachConfirmAriaLabel(t, taskTitle)}
      confirmTestId={DETACH_CONFIRM_TEST_ID}
      focusReturnRef={focusReturnRef ?? anchorRef}
      onOpenChange={onOpenChange}
      onConfirm={onConfirm}
      fallback={fallback}
    />
  );
}

type TaskDetachConfirmDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskTitle?: string;
  sharesParentWorkspace?: boolean;
  isDetaching?: boolean;
  onConfirm: () => void;
};

export function TaskDetachConfirmDialog({
  open,
  onOpenChange,
  taskTitle,
  sharesParentWorkspace,
  isDetaching,
  onConfirm,
}: TaskDetachConfirmDialogProps) {
  const { t } = useTranslation();
  return (
    <AlertDialog
      open={open}
      onOpenChange={(next) => {
        if (!isDetaching) onOpenChange(next);
      }}
    >
      <AlertDialogContent onClick={(event) => event.stopPropagation()}>
        <AlertDialogHeader>
          <AlertDialogTitle>{t(DETACH_TASK_FROM_PARENT_KEY)}</AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div className="min-w-0 space-y-2 text-left">
              <TaskDetachDescription
                taskTitle={taskTitle}
                sharesParentWorkspace={sharesParentWorkspace}
                structured
              />
            </div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={isDetaching} className="cursor-pointer">
            {t(CANCEL_KEY)}
          </AlertDialogCancel>
          <AlertDialogAction
            disabled={isDetaching}
            className="cursor-pointer"
            data-testid={DETACH_CONFIRM_TEST_ID}
            onClick={(event) => {
              event.preventDefault();
              if (!isDetaching) onConfirm();
            }}
          >
            {isDetaching && <IconLoader className="mr-2 h-4 w-4 animate-spin" />}
            {t(DETACH_LABEL_KEY)}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

export function TaskDetachTargetConfirmDialog({
  target,
  detachingTaskId,
  onDismiss,
  onConfirm,
}: {
  target: {
    id: string;
    title: string;
    workspaceMode?: "inherit_parent" | "new_workspace" | "shared_group";
  } | null;
  detachingTaskId: string | null;
  onDismiss: () => void;
  onConfirm: () => void;
}) {
  return (
    <TaskDetachConfirmDialog
      open={target !== null}
      onOpenChange={(open) => {
        if (!open) onDismiss();
      }}
      taskTitle={target?.title}
      sharesParentWorkspace={target?.workspaceMode === "inherit_parent"}
      isDetaching={target?.id === detachingTaskId}
      onConfirm={onConfirm}
    />
  );
}
