"use client";

import { useState } from "react";
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
import { useTranslation } from "react-i18next";
import { useTaskDeletePreflight } from "@/hooks/use-task-delete-preflight";
import {
  canConfirmDeletion,
  DiscardWorktreeChangesOption,
  PreflightStatusBanner,
} from "@/components/task/task-delete-confirm-dialog";
import {
  TASK_CONFIRM_ACTION_CLASS,
  TASK_CONFIRM_BODY_CLASS,
  TASK_CONFIRM_CLASS,
  TASK_CONFIRM_FOOTER_CLASS,
  TASK_CONFIRM_HEADER_CLASS,
} from "@/components/task/task-confirm-dialog-shared";
import type { DeleteTaskParams } from "@/lib/api/domains/kanban-api";

type QuickChatDeleteDialogProps = {
  sessionToDelete: string | null;
  taskId?: string;
  onOpenChange: (open: boolean) => void;
  onConfirm: (options: DeleteTaskParams) => void;
};

export function QuickChatDeleteDialog(props: QuickChatDeleteDialogProps) {
  if (!props.sessionToDelete) return null;
  return (
    <QuickChatDeleteConfirmation
      key={`${props.sessionToDelete}:${props.taskId ?? ""}`}
      {...props}
    />
  );
}

function QuickChatDeleteConfirmation({
  taskId,
  onOpenChange,
  onConfirm,
}: QuickChatDeleteDialogProps) {
  const { t } = useTranslation();
  const [keepFiles, setKeepFiles] = useState(false);
  const [discard, setDiscard] = useState(false);
  const [unverified, setUnverified] = useState(false);
  const preflight = useTaskDeletePreflight(!!taskId, taskId, undefined, false, {
    discardWorktreeChanges: discard,
    workspaceCleanup: keepFiles ? "retain" : "delete",
    allowUnverifiedWorkspaceChanges: unverified,
  });
  const unknown = preflight.workspaceInspection === "unknown";
  const requiresConsent = preflight.status === "resolved" && preflight.requiresDiscardConsent;
  const disabled =
    !!taskId &&
    !canConfirmDeletion(
      false,
      preflight.status === "resolved",
      preflight.confirmationId,
      requiresConsent,
      unknown ? unverified : discard,
    );
  return (
    <AlertDialog open onOpenChange={onOpenChange}>
      <AlertDialogContent size="lg" className={TASK_CONFIRM_CLASS}>
        <AlertDialogHeader className={TASK_CONFIRM_HEADER_CLASS}>
          <AlertDialogTitle>{t("chat:deleteQuickChat")}</AlertDialogTitle>
        </AlertDialogHeader>
        <div className={TASK_CONFIRM_BODY_CLASS}>
          <AlertDialogDescription asChild className="min-w-0 space-y-2 text-left">
            <div>
              <p>{t("chat:deleteQuickChatIntro")}</p>
              {keepFiles ? (
                <p>{t("task:deleteKeepFilesDescription")}</p>
              ) : (
                <ul className="list-disc list-inside space-y-1">
                  <li>{t("chat:allConversationHistory")}</li>
                  <li>{t("chat:theTaskAndItsData")}</li>
                  <li>{t("chat:theAssociatedWorktree")}</li>
                </ul>
              )}
              <p>{t("chat:thisActionCannotBeUndone")}</p>
            </div>
          </AlertDialogDescription>
          {taskId && (
            <>
              <label className="flex items-start gap-2 text-sm max-sm:min-h-11 pointer-coarse:min-h-11">
                <Checkbox
                  checked={keepFiles}
                  data-testid="delete-keep-files-checkbox"
                  onCheckedChange={(checked) => {
                    setKeepFiles(checked === true);
                    setDiscard(false);
                    setUnverified(false);
                  }}
                />
                <span>{t("task:deleteKeepFilesOption")}</span>
              </label>
              <PreflightStatusBanner status={preflight.status} onRetry={preflight.retry} />
              <DiscardWorktreeChangesOption
                unknown={unknown}
                enabled={requiresConsent}
                checked={unknown ? unverified : discard}
                onCheckedChange={(checked) => {
                  setDiscard(checked);
                  setUnverified(unknown && checked);
                }}
              />
            </>
          )}
        </div>
        <AlertDialogFooter className={TASK_CONFIRM_FOOTER_CLASS}>
          <AlertDialogCancel className={TASK_CONFIRM_ACTION_CLASS}>
            {t("common:cancel")}
          </AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={disabled}
            className={TASK_CONFIRM_ACTION_CLASS}
            onClick={() =>
              onConfirm({
                confirmationId: preflight.confirmationId,
                discardWorktreeChanges: discard,
                ...(keepFiles ? { workspaceCleanup: "retain" } : {}),
                ...(unverified ? { allowUnverifiedWorkspaceChanges: true } : {}),
              })
            }
          >
            {t(keepFiles ? "task:deleteKeepFilesAction" : "chat:delete")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
