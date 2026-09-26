"use client";

import { useSessionComposerRecovery } from "./session-recovery-context";
import { useCallback, useState } from "react";
import { IconAlertTriangle, IconInfoCircle } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { sanitizeSessionErrorDetails } from "@/lib/session-error-details";
import { SessionErrorDetails } from "@/components/task/session-error-details";
import { NewSessionDialog } from "@/components/task/new-session-dialog";
import { useSessionRecoveryActions } from "@/hooks/domains/session/use-session-recovery-actions";
import {
  isSessionRecoveryBusy,
  sessionRecoveryOwnerId,
  type SessionRecoveryOwner,
} from "@/lib/session-recovery-presentation";
import { useSessionProfileExists } from "./session-stopped-banner";
import type { TaskStatusSummaryActiveError } from "@/lib/types/task-status-summary";

import { buildRecoveryCardModel } from "./session-bootstrap-recovery-model";
import { RecoveryCardContent } from "./session-bootstrap-recovery-content";

type SessionBootstrapRecoveryCardProps = {
  taskId: string;
  sessionId: string;
  workspaceId?: string | null;
  error: TaskStatusSummaryActiveError;
  automaticRecovery?: SessionRecoveryOwner | null;
};

function BootstrapRecoveryControls({
  taskId,
  sessionId,
  workspaceId,
  error,
  automaticRecovery,
}: SessionBootstrapRecoveryCardProps) {
  const { t } = useTranslation();
  const [showDialog, setShowDialog] = useState(false);
  const {
    busyAction,
    recoveryError,
    manualRecoveryFailure,
    branchDetails,
    guardDetails,
    recoveryNotice,
    handleRecover,
    handleRestore,
    handleNewBranch,
  } = useSessionRecoveryActions({ taskId, sessionId, errorStamp: error.stamp });
  const profileExists = useSessionProfileExists(sessionId);
  const automaticBusy = Boolean(
    automaticRecovery && isSessionRecoveryBusy(automaticRecovery.resumptionState),
  );
  const effectiveBusyAction = automaticBusy ? "resume" : busyAction;
  const manualFailure = manualRecoveryFailure ?? (recoveryError ? { operation: "resume" } : null);
  const model = buildRecoveryCardModel({
    error,
    automaticRecovery,
    manualFailure,
    manualError: recoveryError,
    recoveryNotice,
    translate: t,
  });
  if (guardDetails && recoveryError)
    model.summary = sanitizeSessionErrorDetails(recoveryError.message, 240) || model.summary;
  if (branchDetails) model.summary = t("task:branchIsNoLongerAvailable");
  const copy = {
    launchNeedsAttention: t("task:launchNeedsAttention"),
    launchErrorNoChanges: t("task:launchErrorNoChanges"),
    sessionRecoveryDetails: t("task:sessionRecoveryDetails"),
  };

  const handleResume = useCallback(() => {
    if (!automaticBusy && profileExists) void handleRecover("resume");
  }, [automaticBusy, handleRecover, profileExists]);

  const handleFreshStart = useCallback(() => {
    if (automaticBusy) return;
    if (!profileExists) {
      setShowDialog(true);
      return;
    }
    void handleRecover("fresh_start");
  }, [automaticBusy, handleRecover, profileExists]);

  const cardClassName = model.isReadOnly
    ? "border-blue-500/30 bg-blue-500/5"
    : "border-destructive/30 bg-destructive/5";
  const iconClassName = model.isReadOnly
    ? "bg-blue-500/10 text-blue-600 dark:text-blue-400"
    : "bg-red-500/10 text-red-600 dark:text-red-400";

  return (
    <div
      className={`flex min-w-0 gap-3 rounded-md border p-3 sm:p-4 ${cardClassName}`}
      data-testid="session-bootstrap-recovery-card"
      id={sessionRecoveryOwnerId(automaticRecovery?.recoveryFailure)}
      tabIndex={-1}
      role={model.isReadOnly ? "status" : undefined}
    >
      <div
        className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-md ${iconClassName}`}
      >
        {model.isReadOnly ? (
          <IconInfoCircle className="h-4 w-4" aria-hidden="true" />
        ) : (
          <IconAlertTriangle className="h-4 w-4" aria-hidden="true" />
        )}
      </div>
      <RecoveryCardContent
        model={model}
        error={error}
        profileExists={profileExists}
        busyAction={effectiveBusyAction}
        hasBranchRecovery={branchDetails !== null}
        blocked={Boolean(guardDetails && !guardDetails.retryable)}
        canRestore={!guardDetails}
        onResume={handleResume}
        onRestore={() => void handleRestore()}
        onFreshStart={handleFreshStart}
        onNewBranch={() => void handleNewBranch()}
        copy={copy}
        translate={t}
      />
      <NewSessionDialog
        open={showDialog}
        onOpenChange={setShowDialog}
        taskId={taskId}
        workspaceId={workspaceId}
      />
    </div>
  );
}

export function SessionBootstrapRecoveryCard(props: SessionBootstrapRecoveryCardProps) {
  const owner = useSessionComposerRecovery(props.sessionId);
  const { t } = useTranslation();
  if (owner)
    return (
      <div
        className="min-w-0 py-2 text-xs text-muted-foreground"
        data-testid="session-recovery-history"
      >
        <p>{t("task:sessionBootstrapRecoveryTitle")}</p>
        <SessionErrorDetails>{props.error.details ?? ""}</SessionErrorDetails>
      </div>
    );
  return <BootstrapRecoveryControls {...props} />;
}
