"use client";

import { useState } from "react";
import { Trans, useTranslation } from "react-i18next";
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

/**
 * Never translated. The confirm button is gated on `typed === CONFIRM_TOKEN`
 * and the same token is sent to `resetDatabase`, so a localized copy would
 * leave a dialog the user cannot satisfy in that locale — and the wipe is
 * irreversible. It travels as an interpolated value into the visible sentence
 * and the input's placeholder/aria-label so shown and compared cannot drift.
 */
const CONFIRM_TOKEN = "RESET";

/** A filesystem path shown to the user; interpolated as a value, never translated. */
const BACKUP_DIR_PLACEHOLDER = "<data-dir>/backups/";

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
  const { t } = useTranslation();
  return (
    <>
      <DialogHeader>
        <DialogTitle className="flex items-center gap-2">
          <IconAlertTriangle className="h-5 w-5 text-destructive" />
          {t("system:factoryResetTitle")}
        </DialogTitle>
        <DialogDescription className="space-y-2">
          <span>{t("system:factoryResetWipes")}</span>
          <span className="block">
            <Trans i18nKey="system:factoryResetSnapshot" values={{ path: BACKUP_DIR_PLACEHOLDER }}>
              A snapshot is created automatically under <code>{BACKUP_DIR_PLACEHOLDER}</code> before
              the wipe runs, so you can restore from the Backups page if you change your mind.
            </Trans>
          </span>
          <span className="block font-medium text-foreground">
            <Trans i18nKey="system:factoryResetTypeToConfirm" values={{ token: CONFIRM_TOKEN }}>
              Type <code>{CONFIRM_TOKEN}</code> to enable the confirm button. After the wipe
              completes you&apos;ll be asked to quit and relaunch Kandev - the backend does not
              auto-restart.
            </Trans>
          </span>
        </DialogDescription>
      </DialogHeader>

      <div className="space-y-3">
        <Input
          autoFocus
          placeholder={t("system:factoryResetPlaceholder", { token: CONFIRM_TOKEN })}
          aria-label={t("system:factoryResetPlaceholder", { token: CONFIRM_TOKEN })}
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
            <Spinner className="size-4" /> {t("system:factoryResetWiping")}
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
          {t("common:cancel")}
        </Button>
        <Button
          variant="destructive"
          onClick={onConfirm}
          disabled={!enabled}
          className="cursor-pointer"
          data-testid="system-factory-reset-confirm"
        >
          {t("system:factoryResetTitle")}
        </Button>
      </DialogFooter>
    </>
  );
}

function SuccessView({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  return (
    <>
      <DialogHeader>
        <DialogTitle className="flex items-center gap-2">
          <IconCircleCheck className="h-5 w-5 text-emerald-500" />
          {t("system:factoryResetCompleteTitle")}
        </DialogTitle>
        <DialogDescription>
          <span>{t("system:factoryResetCompleteBody")}</span>
        </DialogDescription>
      </DialogHeader>
      <DialogFooter>
        <Button
          variant="outline"
          onClick={onClose}
          className="cursor-pointer"
          data-testid="system-factory-reset-close"
        >
          {t("system:close")}
        </Button>
      </DialogFooter>
    </>
  );
}

export function FactoryResetDialog({ open, onOpenChange }: Props) {
  const { t } = useTranslation();
  const [typed, setTyped] = useState("");
  const [jobId, setJobId] = useState<string | null>(null);
  const [requestPending, setRequestPending] = useState(false);
  const [requestError, setRequestError] = useState<string | null>(null);

  const job = useSystemJob(jobId);
  const succeeded = job?.state === "succeeded";
  const failed = job?.state === "failed";
  const submitting = requestPending || (jobId !== null && !succeeded && !failed);
  // `job.message` is the backend's own diagnostic text and stays as sent.
  const error = requestError ?? (failed ? (job?.message ?? t("system:factoryResetFailed")) : null);
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
      setRequestError(err instanceof Error ? err.message : t("system:factoryResetRequestFailed"));
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
