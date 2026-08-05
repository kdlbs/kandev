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
import type { PluginRecord } from "@/lib/types/plugins";

type UninstallPluginDialogProps = {
  target: PluginRecord | null;
  busy: boolean;
  onClose: () => void;
  onConfirm: () => void;
};

export function UninstallPluginDialog({
  target,
  busy,
  onClose,
  onConfirm,
}: UninstallPluginDialogProps) {
  const { t } = useTranslation();
  return (
    <Dialog
      open={Boolean(target)}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("plugins:uninstallPlugin")}</DialogTitle>
          <DialogDescription>
            {/* The display name comes from the plugin's manifest, so it is
                third-party data rather than our copy. */}
            <Trans
              i18nKey="plugins:uninstallConfirm"
              values={{ name: target?.display_name ?? t("plugins:thisPlugin") }}
            >
              This will permanently remove{" "}
              <span className="font-medium text-foreground">{target?.display_name}</span> and revoke
              its API key. This action cannot be undone.
            </Trans>
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose} className="cursor-pointer">
            {t("plugins:cancel")}
          </Button>
          <Button
            type="button"
            variant="destructive"
            onClick={onConfirm}
            disabled={busy}
            className="cursor-pointer"
          >
            {t("plugins:confirmUninstall")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
