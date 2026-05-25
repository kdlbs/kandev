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
import { IconAlertTriangle, IconCircleCheck } from "@tabler/icons-react";
import { resetDatabase } from "@/lib/api/domains/system-api";
import { useSystemJob } from "@/hooks/domains/system/use-system-jobs";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

const CONFIRM_TOKEN = "RESET";

function ConfirmView({
  typed,
  onTyped,
  submitting,
  error,
  enabled,
  onCancel,
  onConfirm,
}: {
  typed: string;
  onTyped: (v: string) => void;
  submitting: boolean;
  error: string | null;
  enabled: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  return (
    <>
      <DialogHeader>
        <DialogTitle className="flex items-center gap-2">
          <IconAlertTriangle className="h-5 w-5 text-destructive" />
          Factory Reset
        </DialogTitle>
        <DialogDescription className="space-y-2">
          <span>
            This wipes the entire kandev install: database, worktrees, repo clones, sessions, tasks,
            and quick-chat sessions.
          </span>
          <span className="block">
            A snapshot is created automatically under <code>{`<data-dir>/backups/`}</code> before
            the wipe runs, so you can restore from the Backups page if you change your mind.
          </span>
          <span className="block font-medium text-foreground">
            Type <code>RESET</code> to enable the confirm button. After the wipe completes
            you&apos;ll be asked to quit and relaunch Kandev - the backend does not auto-restart.
          </span>
        </DialogDescription>
      </DialogHeader>

      <div className="space-y-3">
        <Input
          autoFocus
          placeholder="Type RESET to confirm"
          value={typed}
          onChange={(e) => onTyped(e.target.value)}
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
            <Spinner className="size-4" /> Wiping data...
          </div>
        )}
      </div>

      <DialogFooter>
        <Button
          variant="outline"
          onClick={onCancel}
          disabled={submitting}
          className="cursor-pointer"
          data-testid="system-factory-reset-cancel"
        >
          Cancel
        </Button>
        <Button
          variant="destructive"
          onClick={onConfirm}
          disabled={!enabled}
          className="cursor-pointer"
          data-testid="system-factory-reset-confirm"
        >
          Factory Reset
        </Button>
      </DialogFooter>
    </>
  );
}

function SuccessView({ onClose }: { onClose: () => void }) {
  return (
    <>
      <DialogHeader>
        <DialogTitle className="flex items-center gap-2">
          <IconCircleCheck className="h-5 w-5 text-emerald-500" />
          Factory reset complete
        </DialogTitle>
        <DialogDescription>
          <span>
            Kandev&apos;s data has been wiped. Quit and relaunch the app to start fresh - the
            running backend still holds connections to the now-empty database and will not rebuild
            its caches until you restart. The pre-reset snapshot is available on the Backups page if
            you need to roll back.
          </span>
        </DialogDescription>
      </DialogHeader>
      <DialogFooter>
        <Button
          variant="outline"
          onClick={onClose}
          className="cursor-pointer"
          data-testid="system-factory-reset-close"
        >
          Close
        </Button>
      </DialogFooter>
    </>
  );
}

export function FactoryResetDialog({ open, onOpenChange }: Props) {
  const [typed, setTyped] = useState("");
  const [jobId, setJobId] = useState<string | null>(null);
  const [requestPending, setRequestPending] = useState(false);
  const [requestError, setRequestError] = useState<string | null>(null);

  const job = useSystemJob(jobId);
  const succeeded = job?.state === "succeeded";
  const failed = job?.state === "failed";
  const submitting = requestPending || (jobId !== null && !succeeded && !failed);
  const error = requestError ?? (failed ? (job?.message ?? "Reset failed") : null);
  const enabled = typed === CONFIRM_TOKEN && !submitting && !succeeded;

  const handleClose = (next: boolean) => {
    if (submitting) return;
    if (!next) {
      setTyped("");
      setRequestError(null);
      setJobId(null);
    }
    onOpenChange(next);
  };

  const onConfirm = async () => {
    setRequestPending(true);
    setRequestError(null);
    setJobId(null);
    try {
      const res = await resetDatabase(CONFIRM_TOKEN);
      setJobId(res.job_id);
    } catch (err) {
      setRequestError(err instanceof Error ? err.message : "Reset request failed");
    } finally {
      setRequestPending(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent data-testid="system-factory-reset-dialog">
        {succeeded ? (
          <SuccessView onClose={() => handleClose(false)} />
        ) : (
          <ConfirmView
            typed={typed}
            onTyped={setTyped}
            submitting={submitting}
            error={error}
            enabled={enabled}
            onCancel={() => handleClose(false)}
            onConfirm={() => void onConfirm()}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
