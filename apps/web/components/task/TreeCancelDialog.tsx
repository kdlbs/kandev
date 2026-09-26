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

type TreeCancelDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskCount: number;
  activeRunCount: number;
  onConfirm: () => void;
};

export function TreeCancelDialog({
  open,
  onOpenChange,
  taskCount,
  activeRunCount,
  onConfirm,
}: TreeCancelDialogProps) {
  const { t } = useTranslation();
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("task:cancelTaskTree")}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("task:treeCancelDescription", { taskCount, activeRunCount })}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">{t("task:keepRunning")}</AlertDialogCancel>
          <AlertDialogAction className="cursor-pointer" onClick={onConfirm}>
            {t("task:cancelTree")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
