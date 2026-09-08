"use client";

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

type QuickChatDeleteDialogProps = {
  sessionToDelete: string | null;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
};

export function QuickChatDeleteDialog({
  sessionToDelete,
  onOpenChange,
  onConfirm,
}: QuickChatDeleteDialogProps) {
  const { t } = useTranslation();
  return (
    <AlertDialog open={!!sessionToDelete} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("chat:deleteQuickChat")}</AlertDialogTitle>
          <AlertDialogDescription asChild className="min-w-0 space-y-2 text-left">
            <div>
              <p>{t("chat:deleteQuickChatIntro")}</p>
              <ul className="list-disc list-inside space-y-1">
                <li>{t("chat:allConversationHistory")}</li>
                <li>{t("chat:theTaskAndItsData")}</li>
                <li>{t("chat:theAssociatedWorktree")}</li>
              </ul>
              <p>{t("chat:thisActionCannotBeUndone")}</p>
            </div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">{t("common:cancel")}</AlertDialogCancel>
          <AlertDialogAction
            onClick={onConfirm}
            className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            {t("chat:delete")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
