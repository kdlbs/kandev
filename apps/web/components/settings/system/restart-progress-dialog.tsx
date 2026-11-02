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
  formatTimingDuration,
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
  const startupProgress = useStartupProgress(
    phase === "restarting" || phase === "done",
    phase === "done" || phase === "error",
  );
  if (phase === "idle") return null;
  const done = phase === "done";
  const failed = phase === "error";
  return (
    <Dialog open onOpenChange={(open) => !open && (done || failed) && onDismiss()}>
      <DialogContent
        className="flex max-h-[calc(100dvh-2rem)] flex-col overflow-hidden sm:max-w-md"
        data-testid="restart-progress-dialog"
        data-phase={phase}
      >
        <DialogHeader className="shrink-0">
          <DialogTitle className="flex items-center gap-2">
            <RestartStatusIcon phase={phase} />
            {restartTitle(phase, t)}
          </DialogTitle>
          <DialogDescription>{restartDescription(phase, errorMessage, t)}</DialogDescription>
        </DialogHeader>
        {(phase === "restarting" || ((done || failed) && startupProgress.snapshot)) && (
          <StartupProgressDetail
            snapshot={startupProgress.snapshot}
            lastKnown={startupProgress.lastKnown}
            t={t}
          />
        )}
        {(done || failed) && (
          <DialogFooter className="shrink-0">
            <Button
              variant={failed ? "outline" : "default"}
              className="h-12 w-full cursor-pointer sm:h-7 sm:w-auto"
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
    <>
      <div
        className="text-muted-foreground shrink-0 space-y-1 text-sm"
        role="status"
        aria-live="polite"
      >
        <p>{formatPhaseLabel(t, snapshot.phase)}</p>
        <p>{formatElapsed(t, snapshot.elapsed_ms)}</p>
        {snapshot.step && <StartupStepDetail step={snapshot.step} t={t} />}
        {lastKnown && <p className="opacity-60">{t("startup:page.lastKnown")}</p>}
      </div>
      {!!snapshot.completed_steps?.length && (
        <section
          className="min-h-0 overflow-y-auto overscroll-contain text-sm"
          aria-label={t("startup:page.timings")}
          tabIndex={0}
          data-testid="startup-timings"
        >
          <h3 className="mb-2 font-medium">{t("startup:page.timings")}</h3>
          <dl className="space-y-2">
            {snapshot.completed_steps.map((step) => (
              <div
                key={step.activation_seq}
                className="flex flex-wrap justify-between gap-x-3 gap-y-0.5"
              >
                <dt className="min-w-0 break-words">
                  {formatStepLabel(t, step)}
                  {!step.completed && (
                    <span className="text-muted-foreground block">
                      {t("startup:page.interrupted")}
                    </span>
                  )}
                </dt>
                <dd className="text-muted-foreground tabular-nums">
                  {formatTimingDuration(t, step.elapsed_ms)}
                </dd>
              </div>
            ))}
          </dl>
        </section>
      )}
    </>
  );
}

function StartupStepDetail({ step, t }: { step: StartupStepSnapshot; t: TFunction }) {
  const showBar = step.measure === "counted" && (step.total ?? 0) > 0;
  const showEta = step.measure === "counted" && step.eta_ms != null;
  const showStalled = step.stalled && step.since_advance_ms != null;
  return (
    <>
      <p>{formatStepLabel(t, step)}</p>
      <p>{formatElapsed(t, step.elapsed_ms)}</p>
      <p>{formatStepProgress(t, step)}</p>
      {showEta && <p>{formatEta(t, step.eta_ms as number)}</p>}
      {showBar && <Progress value={percentDone(step.done ?? 0, step.total ?? 0)} />}
      {showStalled && <p>{formatStalled(t, step.since_advance_ms as number)}</p>}
    </>
  );
}
