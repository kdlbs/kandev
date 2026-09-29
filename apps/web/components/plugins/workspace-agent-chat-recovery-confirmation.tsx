"use client";

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

export function WorkspaceAgentChatRecoveryConfirmation({
  open,
  onOpenChange,
  onConfirm,
}: {
  open: boolean;
  onOpenChange(open: boolean): void;
  onConfirm(): void;
}) {
  const { t } = useTranslation("plugins");
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent data-testid="managed-chat-recovery-confirmation">
        <AlertDialogHeader>
          <AlertDialogTitle>{t("managedChatConfirmRecoveryTitle")}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("managedChatConfirmRecoveryDescription")}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">{t("managedChatCancel")}</AlertDialogCancel>
          <AlertDialogAction
            className="cursor-pointer"
            data-testid="managed-chat-recover-confirm"
            onClick={onConfirm}
          >
            {t("managedChatRecover")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
