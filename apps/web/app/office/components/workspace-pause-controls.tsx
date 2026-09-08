"use client";

import { useState, type MouseEvent } from "react";
import { useTranslation } from "react-i18next";
import { IconPlayerPause, IconPlayerPlay } from "@tabler/icons-react";
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
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import type { WorkspacePauseActionResult } from "@/hooks/domains/office/use-workspace-pause";

const MAX_REASON_LENGTH = 500;

/**
 * Counts Unicode code points, not UTF-16 code units — matching the
 * backend's utf8.RuneCountInString bound (pause/service.go's
 * maxReasonCodePoints). Plain `string.length` counts UTF-16 code units,
 * so a reason with astral-plane characters (many emoji, some CJK
 * extensions) would be wrongly blocked well under the real backend limit.
 */
export function codePointLength(value: string): number {
  return Array.from(value).length;
}

/**
 * AC-OFFICE-KILL-SWITCH-006.12: pause requires a reason and an explicit
 * confirmation before sending. A plain Dialog (not AlertDialog) so the
 * reason textarea participates in the base dialog's Enter-to-confirm.
 */
export function PauseWorkspaceButton({
  onPause,
  disabled,
}: {
  onPause: (reason: string) => Promise<WorkspacePauseActionResult>;
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const trimmed = reason.trim();
  const canSubmit = trimmed.length > 0 && codePointLength(trimmed) <= MAX_REASON_LENGTH;

  const handleOpenChange = (next: boolean) => {
    setOpen(next);
    if (!next) {
      setReason("");
      setError(null);
    }
  };

  const handleSubmit = async () => {
    if (!canSubmit || submitting) return;
    setSubmitting(true);
    setError(null);
    const result = await onPause(trimmed);
    setSubmitting(false);
    if (result.ok) {
      handleOpenChange(false);
      return;
    }
    if (result.error) setError(result.error);
  };

  return (
    <>
      <Button
        size="sm"
        variant="outline"
        className="min-h-11 cursor-pointer gap-1.5 sm:min-h-0"
        disabled={disabled}
        data-testid="office-pause-workspace-button"
        onClick={() => setOpen(true)}
      >
        <IconPlayerPause className="h-3.5 w-3.5" />
        {t("office:pauseWorkspace")}
      </Button>
      <Dialog open={open} onOpenChange={handleOpenChange}>
        <DialogContent data-testid="office-pause-workspace-dialog">
          <DialogHeader>
            <DialogTitle>{t("office:pauseWorkspaceTitle")}</DialogTitle>
            <DialogDescription>{t("office:pauseWorkspaceDescription")}</DialogDescription>
          </DialogHeader>
          <Textarea
            autoFocus
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder={t("office:pauseReasonPlaceholder")}
            data-testid="office-pause-reason-input"
          />
          {error && (
            <p className="text-sm text-destructive" data-testid="office-pause-error">
              {error}
            </p>
          )}
          <DialogFooter>
            <Button
              variant="outline"
              className="cursor-pointer"
              onClick={() => handleOpenChange(false)}
            >
              {t("common:cancel")}
            </Button>
            <Button
              variant="destructive"
              type="submit"
              className="cursor-pointer"
              disabled={!canSubmit || submitting}
              data-testid="office-pause-confirm-button"
              onClick={handleSubmit}
            >
              {submitting ? t("office:pausing") : t("office:pauseWorkspaceConfirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

/**
 * AC-OFFICE-KILL-SWITCH-006.5: resume requires an explicit confirmation
 * before sending. No reason input — the design marks the resume body
 * optional.
 */
export function ResumeWorkspaceButton({
  onResume,
  disabled,
}: {
  onResume: () => Promise<WorkspacePauseActionResult>;
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const handleOpenChange = (next: boolean) => {
    setOpen(next);
    if (!next) setError(null);
  };

  // AlertDialogAction closes the dialog on click by default; preventDefault
  // keeps it open until the request resolves, since it's async and we need
  // to show an inline error rather than dismiss on a failed resume.
  const handleConfirm = async (e: MouseEvent) => {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    const result = await onResume();
    setSubmitting(false);
    if (result.ok) {
      handleOpenChange(false);
      return;
    }
    if (result.error) setError(result.error);
  };

  return (
    <>
      <Button
        size="sm"
        className="min-h-11 cursor-pointer gap-1.5 sm:min-h-0"
        disabled={disabled}
        data-testid="office-resume-workspace-button"
        onClick={() => setOpen(true)}
      >
        <IconPlayerPlay className="h-3.5 w-3.5" />
        {t("office:resumeWorkspace")}
      </Button>
      <AlertDialog open={open} onOpenChange={handleOpenChange}>
        <AlertDialogContent data-testid="office-resume-workspace-dialog">
          <AlertDialogHeader>
            <AlertDialogTitle>{t("office:resumeWorkspaceTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t("office:resumeWorkspaceDescription")}
            </AlertDialogDescription>
          </AlertDialogHeader>
          {error && (
            <p className="text-sm text-destructive" data-testid="office-resume-error">
              {error}
            </p>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel className="cursor-pointer">{t("common:cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="cursor-pointer"
              disabled={submitting}
              data-testid="office-resume-confirm-button"
              onClick={handleConfirm}
            >
              {submitting ? t("office:resuming") : t("office:resumeWorkspaceConfirm")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
