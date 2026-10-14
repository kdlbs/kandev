"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { formatDateTime } from "@/lib/i18n/formats";
import type { WorkspaceRecoveryProjection } from "@/lib/types/http";
import type { WorkspaceRecoveryStatusCheck } from "@/hooks/domains/session/use-session-recovery-actions";

const PHASE_TRANSLATION_KEYS: Record<string, string> = {
  checking: "task:workspaceRecoveryPhaseChecking",
  snapshotting: "task:workspaceRecoveryPhaseSnapshotting",
  verifying_snapshot: "task:workspaceRecoveryPhaseVerifyingSnapshot",
  restoring: "task:workspaceRecoveryPhaseRestoring",
  verifying_replacement: "task:workspaceRecoveryPhaseVerifyingReplacement",
  publishing: "task:workspaceRecoveryPhasePublishing",
  resuming: "task:workspaceRecoveryPhaseResuming",
};

export function WorkspaceRecoveryProgress({
  projection,
  repositoryName,
  statusCheck = "idle",
  onCheckStatus,
}: {
  projection?: WorkspaceRecoveryProjection | null;
  repositoryName?: string | null;
  statusCheck?: WorkspaceRecoveryStatusCheck;
  onCheckStatus?: () => void;
}) {
  const { t } = useTranslation();
  if (!projection || projection.agent_ready)
    return statusCheckContent(statusCheck, onCheckStatus, t);

  const phaseKey = PHASE_TRANSLATION_KEYS[projection.phase];
  const heading = recoveryHeading(projection, t);
  const phase = phaseKey ? t(phaseKey) : t("task:workspaceRecoveryPhaseUnknown");
  const position =
    projection.repository_position > 0 && projection.repository_total > 0
      ? t("task:workspaceRecoveryRepositoryPosition", {
          position: projection.repository_position,
          total: projection.repository_total,
          repository: repositoryName ?? t("task:workspaceRecoveryRepositoryUnknown"),
        })
      : null;
  const lastUpdate = projection.updated_at ? formatDateTime(projection.updated_at) : null;

  return (
    <section
      className="mb-3 min-w-0 rounded-md border border-border bg-muted/30 p-3 text-sm"
      data-testid="workspace-recovery-progress"
      data-recovery-state={projection.state}
      data-recovery-phase={projection.phase}
    >
      <div role="status" aria-live="polite" aria-atomic="true" className="min-w-0">
        <p className="font-medium">{heading}</p>
        {position && <p className="mt-1 break-words text-muted-foreground">{position}</p>}
        <p className="mt-1 break-words text-muted-foreground">{phase}</p>
      </div>
      {lastUpdate && (
        <p
          className="mt-1 text-xs text-muted-foreground"
          data-testid="workspace-recovery-last-update"
        >
          {t("task:workspaceRecoveryLastUpdate", { date: lastUpdate })}
        </p>
      )}
      <p className="mt-2 break-words text-muted-foreground">
        {t("task:workspaceRecoveryCopiesRetained")}
      </p>
      {projection.workspace_complete && !projection.agent_ready && (
        <p
          className="mt-2 break-words text-muted-foreground"
          data-testid="workspace-recovery-agent-pending"
        >
          {t("task:workspaceRecoveryFilesMovedAgentPending")}
        </p>
      )}
      {statusCheckContent(statusCheck, onCheckStatus, t)}
    </section>
  );
}

function statusCheckContent(
  statusCheck: WorkspaceRecoveryStatusCheck,
  onCheckStatus: (() => void) | undefined,
  t: ReturnType<typeof useTranslation>["t"],
) {
  if (statusCheck === "idle") return null;
  return (
    <div
      className="mt-2 flex min-w-0 flex-wrap items-center gap-2"
      data-testid="workspace-recovery-status-check"
    >
      <p role="status" aria-live="polite" className="min-w-0 flex-1 text-xs text-muted-foreground">
        {statusCheck === "checking"
          ? t("task:workspaceRecoveryStatusChecking")
          : t("task:workspaceRecoveryStatusUnresolved")}
      </p>
      {statusCheck === "unresolved" && onCheckStatus && (
        <Button
          type="button"
          variant="outline"
          onClick={onCheckStatus}
          className={controlSizingClassName("standard", "min-h-11 shrink-0 md:min-h-7")}
          data-testid="workspace-recovery-check-status"
        >
          {t("task:workspaceRecoveryCheckStatus")}
        </Button>
      )}
    </div>
  );
}

function recoveryHeading(
  projection: WorkspaceRecoveryProjection,
  t: ReturnType<typeof useTranslation>["t"],
) {
  if (projection.workspace_complete && !projection.agent_ready)
    return t("task:workspaceRecoveryFilesMovedTitle");
  if (projection.state === "interrupted") return t("task:workspaceRecoveryInterruptedTitle");
  if (projection.state === "failed") return t("task:workspaceRecoveryFailedTitle");
  if (projection.state === "completed") return t("task:workspaceRecoveryCompleteTitle");
  if (projection.state === "running" && projection.phase === "resuming")
    return t("task:workspaceRecoveryResumingTitle");
  if (projection.state === "running") return t("task:workspaceRecoveryMovingTitle");
  return t("task:workspaceRecoveryStatusUnknownTitle");
}
