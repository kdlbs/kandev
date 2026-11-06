"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@kandev/ui/alert-dialog";
import { settingsActionClassName } from "./settings-control";

export function DynamicUnclassifiedBulkApply({ onApply }: { onApply: () => void }) {
  const { t } = useTranslation();
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button
          type="button"
          variant="outline"
          className={settingsActionClassName()}
          data-testid="dynamic-unclassified-apply-all"
        >
          {t("agents:dynamicUnclassifiedApplyAll")}
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("agents:dynamicUnclassifiedApplyAllConfirmTitle")}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("agents:dynamicUnclassifiedApplyAllConfirmDescription")}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("common:cancel")}</AlertDialogCancel>
          <AlertDialogAction
            className={settingsActionClassName()}
            onClick={onApply}
            data-testid="dynamic-unclassified-apply-all-confirm"
          >
            {t("agents:dynamicUnclassifiedApplyAllConfirmAction")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
