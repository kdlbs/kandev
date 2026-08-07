"use client";

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
          <AlertDialogTitle>{t("task:detachTaskFromParent")}</AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div className="space-y-2">
              <p>
                {t("task:detachWillBecomeTopLevel", {
                  taskTitle: taskTitle || t("task:thisTask2"),
                })}
              </p>
              <p>{t("task:detachingChangesTheHierarchyOnlyAccess")}</p>
              {sharesParentWorkspace && (
                <p className="font-medium text-foreground">{t("task:thisTaskSharesItsParentS")}</p>
              )}
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
            {t("task:detach")}
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
