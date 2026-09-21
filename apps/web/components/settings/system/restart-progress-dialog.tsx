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
import { Progress } from "@kandev/ui/progress";
import { Spinner } from "@kandev/ui/spinner";
import { IconAlertTriangle, IconCheck } from "@tabler/icons-react";
import type { KandevRestartPhase } from "@/hooks/domains/system/use-kandev-restart";
import { useStartupProgress } from "@/hooks/domains/system/use-startup-progress";
import {
  formatElapsed,
  formatEta,
  formatPhaseLabel,
  formatStalled,
  formatStepLabel,
  formatStepProgress,
  percentDone,
} from "@/lib/startup-progress/format";
import type { StartupSnapshot, StartupStepSnapshot } from "@/lib/startup-progress/types";

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
  const startupProgress = useStartupProgress(phase === "restarting");
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
        {phase === "restarting" && (
          <StartupProgressDetail
            snapshot={startupProgress.snapshot}
            lastKnown={startupProgress.lastKnown}
            t={t}
          />
        )}
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

type StartupProgressDetailProps = {
  snapshot: StartupSnapshot | null;
  lastKnown: boolean;
  t: TFunction;
};

/** Step-level detail for the "restarting" phase, polled by useStartupProgress (AC-PLATFORM-STARTUP-PROGRESS-003). */
function StartupProgressDetail({ snapshot, lastKnown, t }: StartupProgressDetailProps) {
  if (!snapshot) {
    return (
      <p className="text-muted-foreground text-sm" role="status" aria-live="polite">
        {t("startup:page.waiting")}
      </p>
    );
  }
  return (
    <div className="text-muted-foreground space-y-1 text-sm" role="status" aria-live="polite">
      <p>{formatPhaseLabel(t, snapshot.phase)}</p>
      <p>{formatElapsed(t, snapshot.elapsed_ms)}</p>
      {snapshot.step && <StartupStepDetail step={snapshot.step} t={t} />}
      {lastKnown && <p className="opacity-60">{t("startup:page.lastKnown")}</p>}
    </div>
  );
}

function StartupStepDetail({ step, t }: { step: StartupStepSnapshot; t: TFunction }) {
  const showBar = step.measure === "counted" && (step.total ?? 0) > 0;
  const showEta = step.measure === "counted" && step.eta_ms != null;
  const showStalled = step.stalled && step.since_advance_ms != null;
  return (
    <>
      <p>{formatStepLabel(t, step)}</p>
      <p>{formatStepProgress(t, step)}</p>
      {showEta && <p>{formatEta(t, step.eta_ms as number)}</p>}
      {showBar && <Progress value={percentDone(step.done ?? 0, step.total ?? 0)} />}
      {showStalled && <p>{formatStalled(t, step.since_advance_ms as number)}</p>}
    </>
  );
}
