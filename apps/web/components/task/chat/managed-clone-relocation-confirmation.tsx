"use client";

import type { ReactNode } from "react";
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

export function ManagedCloneRelocationConfirmation({
  open,
  targetKey,
  onOpenChange,
  onConfirm,
  disabled = false,
}: {
  open: boolean;
  targetKey: string;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void | Promise<unknown>;
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  const description: ReactNode = (
    <div className="space-y-3 text-sm leading-6">
      <p>{t("task:managedCloneRelocationConfirmBody")}</p>
      <p className="font-medium text-foreground">
        {t("task:managedCloneRelocationConfirmWarning")}
      </p>
    </div>
  );
  return (
    <MobileActionConfirmation
      open={open}
      targetKey={targetKey}
      onOpenChange={onOpenChange}
      onConfirm={() => {
        void onConfirm();
      }}
      title={t("task:managedCloneRelocationConfirmTitle")}
      description={description}
      cancelLabel={t("common:cancel")}
      confirmLabel={t("task:managedCloneRelocationConfirm")}
      confirmTestId="managed-clone-relocation-confirm"
      testId="managed-clone-relocation-confirmation"
      disabled={disabled}
      completionPolicy="close-before-dispatch"
      fallback={
        <AlertDialog open={open} onOpenChange={onOpenChange}>
          <AlertDialogContent data-testid="managed-clone-relocation-confirmation">
            <AlertDialogHeader>
              <AlertDialogTitle>{t("task:managedCloneRelocationConfirmTitle")}</AlertDialogTitle>
              <AlertDialogDescription asChild>{description}</AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel disabled={disabled} className="cursor-pointer">
                {t("common:cancel")}
              </AlertDialogCancel>
              <AlertDialogAction
                disabled={disabled}
                data-testid="managed-clone-relocation-confirm"
                className="cursor-pointer"
                onClick={() => void onConfirm()}
              >
                {t("task:managedCloneRelocationConfirm")}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      }
    />
  );
}
