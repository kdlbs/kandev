"use client";

import { Trans, useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import type { SecretListItem } from "@/lib/types/http-secrets";

type DeleteSecretDialogProps = {
  target: SecretListItem | null;
  onClose: () => void;
  onConfirm: () => void;
  isBusy: boolean;
};

/** Renders the confirm-delete dialog for a secret, with cancel and destructive confirm actions. */
export function DeleteSecretDialog({
  target,
  onClose,
  onConfirm,
  isBusy,
}: DeleteSecretDialogProps) {
  const { t } = useTranslation();
  // The secret's own name is user data: it is interpolated as a value and never
  // routed through a catalog key.
  const name = target?.name ?? t("settings:thisSecret");
  return (
    <Dialog
      open={Boolean(target)}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("settings:deleteSecret")}</DialogTitle>
          <DialogDescription>
            <Trans i18nKey="settings:thisWillPermanentlyRemoveSecret" values={{ name }}>
              This will permanently remove{" "}
              <span className="font-medium text-foreground">{name}</span>. This action cannot be
              undone.
            </Trans>
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose} className="cursor-pointer">
            {t("settings:cancel")}
          </Button>
          <Button
            type="button"
            variant="destructive"
            onClick={onConfirm}
            disabled={isBusy}
            className="cursor-pointer"
          >
            {t("settings:deleteSecret")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
