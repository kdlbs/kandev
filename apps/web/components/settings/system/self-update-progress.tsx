"use client";

import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Spinner } from "@kandev/ui/spinner";
import { IconCheck, IconAlertTriangle } from "@tabler/icons-react";
import type { SelfUpdatePhase } from "@/hooks/domains/system/use-self-update";

type SelfUpdateProgressProps = {
  phase: SelfUpdatePhase;
  targetVersion: string | null;
  errorMessage: string | null;
  onDismiss: () => void;
};

/**
 * The version string is a value; only the frame around it is copy. When the
 * target version is not known yet the whole sentence switches to a variant
 * that does not name one, rather than interpolating a translated noun phrase.
 */
function activeText(phase: SelfUpdatePhase, target: string | null, t: TFunction): string {
  switch (phase) {
    case "starting":
      return target
        ? t("system:selfUpdateStarting", { version: target })
        : t("system:selfUpdateStartingUnknown");
    case "installing":
      return target
        ? t("system:selfUpdateInstalling", { version: target })
        : t("system:selfUpdateInstallingUnknown");
    case "restarting":
      return t("system:selfUpdateRestarting");
    default:
      return "";
  }
}

function ActiveRow({
  phase,
  targetVersion,
}: {
  phase: SelfUpdatePhase;
  targetVersion: string | null;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1">
      <div className="flex items-center gap-2 text-sm">
        <Spinner className="size-4" />
        <span>{activeText(phase, targetVersion, t)}</span>
      </div>
      <p className="text-xs text-muted-foreground">{t("system:selfUpdateKeepOpen")}</p>
    </div>
  );
}

function DoneRow({ targetVersion }: { targetVersion: string | null }) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
      <div className="flex items-center gap-2 text-sm">
        <IconCheck className="size-4 text-emerald-500" />
        <span>
          {targetVersion
            ? t("system:selfUpdateDone", { version: targetVersion })
            : t("system:selfUpdateDoneUnknown")}
        </span>
      </div>
      <Button
        variant="outline"
        size="sm"
        className="cursor-pointer self-start sm:self-auto"
        onClick={() => window.location.reload()}
        data-testid="system-updates-progress-reload"
      >
        {t("system:reloadPage")}
      </Button>
    </div>
  );
}

function ErrorRow({ message, onDismiss }: { message: string | null; onDismiss: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
      <div className="flex items-start gap-2 text-sm text-destructive">
        <IconAlertTriangle className="size-4 shrink-0" />
        {/* `message` comes from the update API and stays as sent. */}
        <span>{message ?? t("system:selfUpdateFailed")}</span>
      </div>
      <Button
        variant="outline"
        size="sm"
        className="cursor-pointer self-start sm:self-auto"
        onClick={onDismiss}
        data-testid="system-updates-progress-dismiss"
      >
        {t("system:dismiss")}
      </Button>
    </div>
  );
}

function ProgressBody({ phase, targetVersion, errorMessage, onDismiss }: SelfUpdateProgressProps) {
  if (phase === "done") return <DoneRow targetVersion={targetVersion} />;
  if (phase === "error") return <ErrorRow message={errorMessage} onDismiss={onDismiss} />;
  return <ActiveRow phase={phase} targetVersion={targetVersion} />;
}

export function SelfUpdateProgress(props: SelfUpdateProgressProps) {
  if (props.phase === "idle") return null;
  return (
    <div
      className="rounded-md border bg-muted/30 px-3 py-2"
      data-testid="system-updates-progress"
      data-phase={props.phase}
    >
      <ProgressBody {...props} />
    </div>
  );
}
