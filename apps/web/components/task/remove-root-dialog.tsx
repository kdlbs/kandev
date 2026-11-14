"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
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
import { MobileActionConfirmation } from "@/components/confirmation/mobile-action-confirmation";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { detachTaskRepository } from "@/lib/api/domains/kanban-api";
import type { TaskRepositoryRoot } from "@/hooks/domains/session/use-task-repository-roots";

type RemoveRootDialogProps = {
  open: boolean;
  root: TaskRepositoryRoot | null;
  onOpenChange: (open: boolean) => void;
  onRemoved?: () => void;
};

export function RemoveRootDialog({ open, root, onOpenChange, onRemoved }: RemoveRootDialogProps) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const { isFinePointer } = useResponsiveBreakpoint();
  const clearGitRepositoryStatus = useAppStore((state) => state.clearGitRepositoryStatus);
  const [pending, setPending] = useState(false);
  const title = t("task:removeRootTitle", { label: root?.displayLabel ?? "" });
  const body = t("task:removeRootBody");

  const confirm = async () => {
    if (!root) return;
    setPending(true);
    try {
      await detachTaskRepository(root.taskId, root.taskRepositoryId);
      if (root.trackerName) {
        clearGitRepositoryStatus(root.sessionId, root.environmentId, root.trackerName);
      }
      onOpenChange(false);
      onRemoved?.();
    } catch (error) {
      toast({
        title: t("task:removeRootFailed"),
        description: error instanceof Error ? error.message : t("common:requestFailed"),
        variant: "error",
      });
    } finally {
      setPending(false);
    }
  };

  if (!isFinePointer) {
    return (
      <MobileActionConfirmation
        open={open && !!root}
        targetKey={root?.key ?? "root"}
        title={title}
        subject={root?.displayLabel}
        description={body}
        cancelLabel={t("common:cancel")}
        confirmLabel={t("task:removeFromTask")}
        confirmTestId="remove-root-confirm"
        onOpenChange={onOpenChange}
        onConfirm={confirm}
      />
    );
  }

  return (
    <AlertDialog open={open && !!root} onOpenChange={onOpenChange}>
      <AlertDialogContent data-testid="remove-root-dialog">
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>{body}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("common:cancel")}</AlertDialogCancel>
          <AlertDialogAction
            data-testid="remove-root-confirm"
            disabled={pending}
            onClick={(event) => {
              event.preventDefault();
              void confirm();
            }}
          >
            {t("task:removeFromTask")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
