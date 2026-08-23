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
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

const DETACH_TASK_FROM_PARENT_KEY = "task:detachTaskFromParent";
const DETACH_LABEL_KEY = "task:detach";

export type TaskDetachConfirmationCopyProps = {
  taskTitle?: string;
  sharesParentWorkspace?: boolean;
};

function TaskDetachDescription({
  taskTitle,
  sharesParentWorkspace,
}: TaskDetachConfirmationCopyProps): ReactNode {
  const { t } = useTranslation();
  return (
    <span className="block space-y-2">
      <span className="block">
        {t("task:detachWillBecomeTopLevel", {
          taskTitle: taskTitle || t("task:thisTask2"),
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
      cancelLabel={t("common:cancel")}
      confirmLabel={t(DETACH_LABEL_KEY)}
      confirmAriaLabel={detachConfirmAriaLabel(t, taskTitle)}
      confirmTestId="detach-task-confirm"
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
      cancelLabel={t("common:cancel")}
      confirmLabel={t(DETACH_LABEL_KEY)}
      confirmAriaLabel={detachConfirmAriaLabel(t, taskTitle)}
      confirmTestId="detach-task-confirm"
      onCancel={onCancel}
      onClose={onClose}
      onConfirm={onConfirm}
    />
  );
}

export type TaskDetachConfirmationSurfaceProps = TaskDetachConfirmationCopyProps & {
  open: boolean;
  anchorRef: RefObject<HTMLElement | null>;
  focusReturnRef?: RefObject<HTMLElement | null>;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void | Promise<void>;
};

export function TaskDetachConfirmationSurface({
  open,
  anchorRef,
  focusReturnRef,
  taskTitle,
  sharesParentWorkspace,
  onOpenChange,
  onConfirm,
}: TaskDetachConfirmationSurfaceProps) {
  const { isFinePointer } = useResponsiveBreakpoint();
  if (isFinePointer) {
    return (
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
    );
  }
  if (!open) return null;
  return (
    <TaskDetachInlineConfirmation
      taskTitle={taskTitle}
      sharesParentWorkspace={sharesParentWorkspace}
      onCancel={() => onOpenChange(false)}
      onClose={() => onOpenChange(false)}
      onConfirm={onConfirm}
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
            <div>
              <TaskDetachDescription
                taskTitle={taskTitle}
                sharesParentWorkspace={sharesParentWorkspace}
              />
            </div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={isDetaching} className="cursor-pointer">
            {t("common:cancel")}
          </AlertDialogCancel>
          <AlertDialogAction
            disabled={isDetaching}
            className="cursor-pointer"
            data-testid="detach-task-confirm"
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
