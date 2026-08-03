"use client";

import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Spinner } from "@kandev/ui/spinner";
import { IconAlertTriangle, IconCheck } from "@tabler/icons-react";
import type { KandevRestartPhase } from "@/hooks/domains/system/use-kandev-restart";

type RestartProgressDialogProps = {
  phase: KandevRestartPhase;
  errorMessage: string | null;
  onDismiss: () => void;
};

export function RestartProgressDialog({
  phase,
  errorMessage,
  onDismiss,
}: RestartProgressDialogProps) {
  const { t } = useTranslation();
  if (phase === "idle") return null;
  const done = phase === "done";
  const failed = phase === "error";
  return (
    <Dialog open onOpenChange={(open) => !open && (done || failed) && onDismiss()}>
      <DialogContent
        className="sm:max-w-md"
        data-testid="restart-progress-dialog"
        data-phase={phase}
      >
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <RestartStatusIcon phase={phase} />
            {restartTitle(phase, t)}
          </DialogTitle>
          <DialogDescription>{restartDescription(phase, errorMessage, t)}</DialogDescription>
        </DialogHeader>
        {(done || failed) && (
          <DialogFooter>
            <Button
              variant={failed ? "outline" : "default"}
              className="w-full cursor-pointer sm:w-auto"
              onClick={done ? () => window.location.reload() : onDismiss}
            >
              {done ? t("system:reloadPage") : t("system:dismiss")}
            </Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  );
}

function RestartStatusIcon({ phase }: { phase: KandevRestartPhase }) {
  if (phase === "done") return <IconCheck className="size-4 text-emerald-500" />;
  if (phase === "error") return <IconAlertTriangle className="size-4 text-destructive" />;
  return <Spinner className="size-4" />;
}

/** `phase` is a wire enum; only these labels are copy. */
function restartTitle(phase: KandevRestartPhase, t: TFunction): string {
  switch (phase) {
    case "starting":
      return t("system:restartPhaseStartingTitle");
    case "restarting":
      return t("system:restartPhaseRestartingTitle");
    case "done":
      return t("system:restartPhaseDoneTitle");
    case "error":
      return t("system:restartPhaseErrorTitle");
    default:
      return t("system:restartPhaseRestartingTitle");
  }
}

function restartDescription(
  phase: KandevRestartPhase,
  errorMessage: string | null,
  t: TFunction,
): string {
  switch (phase) {
    case "starting":
      return t("system:restartPhaseStartingBody");
    case "restarting":
      return t("system:restartPhaseRestartingBody");
    case "done":
      return t("system:restartPhaseDoneBody");
    case "error":
      // `errorMessage` originates from the restart API and stays as sent.
      return errorMessage ?? t("system:restartPhaseErrorBody");
    default:
      return "";
  }
}
