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

type TaskLspRestartDialogProps = {
  language: string | null;
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
};

export function TaskLspRestartDialog({
  language,
  pending,
  onOpenChange,
  onConfirm,
}: TaskLspRestartDialogProps) {
  const { t } = useTranslation();
  return (
    <AlertDialog open={language !== null} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {t("lsp:restartDialogTitle", { language: language ?? "" })}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {t("lsp:restartDialogDescription", { language: language ?? "" })}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer" disabled={pending}>
            {t("common:cancel")}
          </AlertDialogCancel>
          <AlertDialogAction className="cursor-pointer" disabled={pending} onClick={onConfirm}>
            {t("lsp:restart")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
