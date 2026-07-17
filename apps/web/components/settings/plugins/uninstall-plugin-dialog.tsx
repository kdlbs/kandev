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
  return (
    <Dialog
      open={Boolean(target)}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Uninstall plugin</DialogTitle>
          <DialogDescription>
            This will permanently remove{" "}
            <span className="font-medium text-foreground">
              {target?.display_name ?? "this plugin"}
            </span>{" "}
            and revoke its API key. This action cannot be undone.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose} className="cursor-pointer">
            Cancel
          </Button>
          <Button
            type="button"
            variant="destructive"
            onClick={onConfirm}
            disabled={busy}
            className="cursor-pointer"
          >
            Confirm uninstall
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
