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

type AutomationDeleteConfirmDialogProps = {
  open: boolean;
  automationName: string;
  isDeleting?: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void | Promise<void>;
};

export function AutomationDeleteConfirmDialog({
  open,
  automationName,
  isDeleting = false,
  onOpenChange,
  onConfirm,
}: AutomationDeleteConfirmDialogProps) {
  const { t } = useTranslation();

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent
        data-testid="automation-delete-confirm-dialog"
        className="w-[calc(100vw-2rem)] sm:max-w-sm"
      >
        <AlertDialogHeader>
          <AlertDialogTitle>{t("automations:deleteAutomationTitle")}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("automations:deleteAutomationDescription", { name: automationName })}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={isDeleting} className="w-full cursor-pointer sm:w-auto">
            {t("common:cancel")}
          </AlertDialogCancel>
          <Button
            type="button"
            variant="destructive"
            disabled={isDeleting}
            data-testid="automation-delete-confirm"
            className="w-full cursor-pointer sm:w-auto"
            onClick={onConfirm}
          >
            {t("automations:delete")}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
