"use client";

import { useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Spinner } from "@kandev/ui/spinner";
import { IconAlertTriangle } from "@tabler/icons-react";
import { restoreBackup } from "@/lib/api/domains/system-api";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  name: string;
};

const CONFIRM_TOKEN = "RESTORE";

export function RestoreDialog({ open, onOpenChange, name }: Props) {
  const [typed, setTyped] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const enabled = typed === CONFIRM_TOKEN && !submitting;

  const handleClose = (next: boolean) => {
    if (submitting) return;
    if (!next) {
      setTyped("");
      setError(null);
    }
    onOpenChange(next);
  };

  const onConfirm = async () => {
    setSubmitting(true);
    setError(null);
    try {
      await restoreBackup(name, CONFIRM_TOKEN);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Restore request failed");
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent data-testid="system-restore-dialog">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <IconAlertTriangle className="h-5 w-5 text-destructive" />
            Restore snapshot
          </DialogTitle>
          <DialogDescription className="space-y-2">
            <span>
              Restore <code className="font-mono">{name}</code> over the current database. The
              backend restarts on success; in-flight sessions are interrupted.
            </span>
            <span className="block font-medium text-foreground">
              Type <code>RESTORE</code> to enable the confirm button.
            </span>
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <Input
            autoFocus
            placeholder="Type RESTORE to confirm"
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
            disabled={submitting}
            data-testid="system-restore-input"
          />
          {error && (
            <p className="text-xs text-destructive" data-testid="system-restore-error">
              {error}
            </p>
          )}
          {submitting && (
            <div
              className="flex items-center gap-2 text-sm text-muted-foreground"
              data-testid="system-restore-pending"
            >
              <Spinner className="size-4" /> Restoring... waiting for backend to restart.
            </div>
          )}
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => handleClose(false)}
            disabled={submitting}
            className="cursor-pointer"
            data-testid="system-restore-cancel"
          >
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={() => void onConfirm()}
            disabled={!enabled}
            className="cursor-pointer"
            data-testid="system-restore-confirm"
          >
            Restore
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
