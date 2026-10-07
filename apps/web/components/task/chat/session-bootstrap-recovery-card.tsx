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
import { ManagedCloneRelocationConfirmation } from "./managed-clone-relocation-confirmation";
import { useOptionalAppStore } from "@/components/state-provider";

type SessionBootstrapRecoveryCardProps = {
  taskId: string;
  sessionId: string;
  workspaceId?: string | null;
  error: TaskStatusSummaryActiveError;
  automaticRecovery?: SessionRecoveryOwner | null;
};

type BootstrapRecoveryViewProps = {
  props: SessionBootstrapRecoveryCardProps;
  recovery: ReturnType<typeof useSessionRecoveryActions>;
  model: ReturnType<typeof buildRecoveryCardModel>;
  profileExists: boolean;
  providerRestoredResumeEligible: boolean;
  effectiveBusyAction: ReturnType<typeof useSessionRecoveryActions>["busyAction"];
  needsManagedCloneRelocation: boolean;
  showDialog: boolean;
  setShowDialog: (open: boolean) => void;
  relocationConfirmationOpen: boolean;
  setRelocationConfirmationOpen: (open: boolean) => void;
  handleResume: () => void;
  handleFreshStart: () => void;
  copy: {
    launchNeedsAttention: string;
    launchErrorNoChanges: string;
    sessionRecoveryDetails: string;
  };
  t: ReturnType<typeof useTranslation>["t"];
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
  const [relocationConfirmationOpen, setRelocationConfirmationOpen] = useState(false);
  const agentDisplayName = useOptionalAppStore((state) => {
    const session = state.taskSessions.items[sessionId];
    const profileId = session?.execution_profile_id || session?.agent_profile_id;
    const profile = profileId
      ? state.agentProfiles.items.find((candidate) => candidate.id === profileId)
      : undefined;
    return profile
      ? state.availableAgents.items.find((agent) => agent.name === profile.agent_name)?.display_name
      : undefined;
  }, undefined);
  const recovery = useSessionRecoveryActions({ taskId, sessionId, errorStamp: error.stamp });
  const {
    busyAction,
    recoveryError,
    manualRecoveryFailure,
    branchDetails,
    guardDetails,
    recoveryNotice,
    managedCloneRecoveryStamp,
    handleRecover,
  } = recovery;
  const needsManagedCloneRelocation =
    recovery.workspaceRecovery?.kind === "managed_clone_relocation" ||
    error.category === "managed_clone_relocation_required" ||
    managedCloneRecoveryStamp !== null;
  const profileExists = useSessionProfileExists(sessionId);
  const automaticBusy = Boolean(
    automaticRecovery && isSessionRecoveryBusy(automaticRecovery.resumptionState),
  );
  const effectiveBusyAction = automaticBusy ? "resume" : busyAction;
  const model = buildRecoveryCardModel({
    error,
    automaticRecovery,
    manualFailure: manualRecoveryFailure,
    manualError: recoveryError,
    recoveryNotice,
    translate: t,
    agentDisplayName,
  });
  if (guardDetails && recoveryError) {
    model.summary = sanitizeSessionErrorDetails(recoveryError.message, 240) || model.summary;
    model.showSummary = true;
  }
  if (branchDetails) {
    model.summary = t("task:branchIsNoLongerAvailable");
    model.showSummary = true;
  }
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

  return (
    <BootstrapRecoveryView
      props={{ taskId, sessionId, workspaceId, error, automaticRecovery }}
      recovery={recovery}
      model={model}
      profileExists={profileExists}
      providerRestoredResumeEligible={recovery.providerRestoredResumeEligible}
      effectiveBusyAction={effectiveBusyAction}
      needsManagedCloneRelocation={needsManagedCloneRelocation}
      showDialog={showDialog}
      setShowDialog={setShowDialog}
      relocationConfirmationOpen={relocationConfirmationOpen}
      setRelocationConfirmationOpen={setRelocationConfirmationOpen}
      handleResume={handleResume}
      handleFreshStart={handleFreshStart}
      copy={copy}
      t={t}
    />
  );
}

function BootstrapRecoveryView({
  props,
  recovery,
  model,
  profileExists,
  providerRestoredResumeEligible,
  effectiveBusyAction,
  needsManagedCloneRelocation,
  showDialog,
  setShowDialog,
  relocationConfirmationOpen,
  setRelocationConfirmationOpen,
  handleResume,
  handleFreshStart,
  copy,
  t,
}: BootstrapRecoveryViewProps) {
  if (needsManagedCloneRelocation && recovery.workspaceRecovery?.agent_ready) return null;
  const { taskId, sessionId, workspaceId, error, automaticRecovery } = props;
  const { busyAction, branchDetails, guardDetails, managedCloneRecoveryStamp } = recovery;
  const cardClassName = recoveryCardClassName(model.isReadOnly);
  const iconClassName = recoveryIconClassName(model.isReadOnly);
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
        providerRestoredResumeEligible={providerRestoredResumeEligible}
        busyAction={effectiveBusyAction}
        hasBranchRecovery={branchDetails !== null}
        blocked={Boolean(guardDetails && !guardDetails.retryable)}
        canRestore={!guardDetails}
        needsManagedCloneRelocation={needsManagedCloneRelocation}
        workspaceRecovery={needsManagedCloneRelocation ? recovery.workspaceRecovery : null}
        workspaceRecoveryRepositoryName={recovery.workspaceRecoveryRepositoryName}
        workspaceRecoveryStatusCheck={
          needsManagedCloneRelocation ? recovery.workspaceRecoveryStatusCheck : "idle"
        }
        onCheckWorkspaceRecoveryStatus={() => void recovery.checkWorkspaceRecoveryStatus()}
        onResume={handleResume}
        onRestore={() => void recovery.handleRestore()}
        onFreshStart={handleFreshStart}
        onNewBranch={() => void recovery.handleNewBranch()}
        onRelocate={() => setRelocationConfirmationOpen(true)}
        copy={copy}
        t={t}
      />
      <NewSessionDialog
        open={showDialog}
        onOpenChange={setShowDialog}
        taskId={taskId}
        workspaceId={workspaceId}
      />
      <ManagedCloneRelocationConfirmation
        open={relocationConfirmationOpen}
        targetKey={`${sessionId}:${managedCloneRecoveryStamp ?? error.stamp}`}
        onOpenChange={setRelocationConfirmationOpen}
        onConfirm={() => recovery.handleManagedCloneRelocation()}
        disabled={busyAction !== null}
      />
    </div>
  );
}

function recoveryCardClassName(readOnly: boolean) {
  return readOnly ? "border-blue-500/30 bg-blue-500/5" : "border-destructive/30 bg-destructive/5";
}

function recoveryIconClassName(readOnly: boolean) {
  return readOnly
    ? "bg-blue-500/10 text-blue-600 dark:text-blue-400"
    : "bg-red-500/10 text-red-600 dark:text-red-400";
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
