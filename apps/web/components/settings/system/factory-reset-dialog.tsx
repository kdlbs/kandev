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
import { resetDatabase } from "@/lib/api/domains/system-api";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

const CONFIRM_TOKEN = "RESET";

export function FactoryResetDialog({ open, onOpenChange }: Props) {
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
      await resetDatabase(CONFIRM_TOKEN);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Reset request failed");
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent data-testid="system-factory-reset-dialog">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <IconAlertTriangle className="h-5 w-5 text-destructive" />
            Factory Reset
          </DialogTitle>
          <DialogDescription className="space-y-2">
            <span>
              This wipes the entire kandev install: database, worktrees, repo clones, sessions,
              tasks, and quick-chat sessions.
            </span>
            <span className="block">
              A snapshot is created automatically under <code>{`<data-dir>/backups/`}</code> before
              the wipe runs, so you can restore from the Backups page if you change your mind.
            </span>
            <span className="block font-medium text-foreground">
              Type <code>RESET</code> to enable the confirm button. The backend restarts on success.
            </span>
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <Input
            autoFocus
            placeholder="Type RESET to confirm"
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
            disabled={submitting}
            data-testid="system-factory-reset-input"
          />
          {error && (
            <p className="text-xs text-destructive" data-testid="system-factory-reset-error">
              {error}
            </p>
          )}
          {submitting && (
            <div
              className="flex items-center gap-2 text-sm text-muted-foreground"
              data-testid="system-factory-reset-pending"
            >
              <Spinner className="size-4" /> Resetting... waiting for backend to restart.
            </div>
          )}
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => handleClose(false)}
            disabled={submitting}
            className="cursor-pointer"
            data-testid="system-factory-reset-cancel"
          >
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={() => void onConfirm()}
            disabled={!enabled}
            className="cursor-pointer"
            data-testid="system-factory-reset-confirm"
          >
            Factory Reset
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
