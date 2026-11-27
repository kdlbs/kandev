"use client";

import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";

type CoordinatorDeleteConfirmDialogProps = {
  open: boolean;
  coordinatorName: string;
  isDeleting?: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void | Promise<void>;
};

// Design B13: a feature-owned AlertDialog following the automation
// delete-confirm pattern (no generic shared confirmation component exists).
export function CoordinatorDeleteConfirmDialog({
  open,
  coordinatorName,
  isDeleting = false,
  onOpenChange,
  onConfirm,
}: CoordinatorDeleteConfirmDialogProps) {
  const { t } = useTranslation();

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent
        data-testid="coordinator-delete-confirm-dialog"
        className="w-[calc(100vw-2rem)] sm:max-w-sm"
      >
        <AlertDialogHeader>
          <AlertDialogTitle>{t("coordinator:deleteCoordinator")}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("coordinator:deleteCoordinatorDescription", { name: coordinatorName })}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel
            disabled={isDeleting}
            className="min-h-12 w-full cursor-pointer sm:min-h-9 sm:w-auto"
          >
            {t("common:cancel")}
          </AlertDialogCancel>
          <Button
            type="button"
            variant="destructive"
            disabled={isDeleting}
            data-testid="coordinator-delete-confirm"
            className="min-h-12 w-full cursor-pointer sm:min-h-9 sm:w-auto"
            onClick={onConfirm}
          >
            {t("common:delete")}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
