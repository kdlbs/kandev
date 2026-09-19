"use client";

import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import type { TaskPlanRevision } from "@/lib/types/http";
import { useTranslation } from "react-i18next";

export function RevertConfirmDialog({
  target,
  isSaving,
  onCancel,
  onConfirm,
}: {
  target: TaskPlanRevision | null;
  isSaving: boolean;
  onCancel: () => void;
  onConfirm: () => void | Promise<void>;
}) {
  const { t } = useTranslation();
  const open = target !== null;
  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!v) onCancel();
      }}
    >
      <DialogContent data-testid="plan-revert-confirm-dialog">
        <DialogHeader>
          <DialogTitle>
            {t("task:restoreToVersionConfirm", { version: target?.revision_number })}
          </DialogTitle>
          <DialogDescription>
            {t("task:restoreToVersionDescription", { version: target?.revision_number })}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            variant="outline"
            onClick={onCancel}
            disabled={isSaving}
            className="cursor-pointer"
            data-testid="plan-revert-confirm-cancel"
          >
            {t("common:cancel")}
          </Button>
          <Button
            onClick={onConfirm}
            disabled={isSaving}
            className="cursor-pointer"
            data-testid="plan-revert-confirm-ok"
          >
            {isSaving ? t("task:restoring") : t("task:restore")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
