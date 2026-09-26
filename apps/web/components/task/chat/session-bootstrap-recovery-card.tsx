"use client";

import { useSessionComposerRecovery } from "./session-recovery-context";
import { useCallback, useState } from "react";
import { IconAlertTriangle, IconInfoCircle } from "@tabler/icons-react";
import { RecoveryActions, type RecoveryChoice } from "@/components/task/recovery-actions";
import { useTranslation } from "react-i18next";
import { sanitizeSessionErrorDetails } from "@/lib/session-error-details";
import { SessionErrorDetails } from "@/components/task/session-error-details";
import { NewSessionDialog } from "@/components/task/new-session-dialog";
import {
  type ManualSessionRecoveryFailure,
  useSessionRecoveryActions,
  type SessionRecoveryBusyAction,
} from "@/hooks/domains/session/use-session-recovery-actions";
import {
  isSessionRecoveryBusy,
  sessionRecoveryOwnerId,
  type SessionRecoveryOwner,
} from "@/lib/session-recovery-presentation";
import { useSessionProfileExists } from "./session-stopped-banner";
import type {
  AgentErrorCause,
  TaskStatusSummaryActiveError,
} from "@/lib/types/task-status-summary";

type SessionBootstrapRecoveryCardProps = {
  taskId: string;
  sessionId: string;
  workspaceId?: string | null;
  error: TaskStatusSummaryActiveError;
  automaticRecovery?: SessionRecoveryOwner | null;
};

export function causeLabel(code: string | undefined, translate: (key: string) => string): string {
  switch (code) {
    case "authentication_required":
      return translate("task:sessionBootstrapCauseAuthenticationRequired");
    case "permission_denied":
      return translate("task:sessionBootstrapCausePermissionDenied");
    case "destination_invalid":
      return translate("task:sessionBootstrapCauseDestinationInvalid");
    case "source_branch_missing":
      return translate("task:sessionBootstrapCauseSourceBranchMissing");
    case "transport_unavailable":
      return translate("task:sessionBootstrapCauseTransportUnavailable");
    case "timeout":
      return translate("task:sessionBootstrapCauseTimeout");
    default:
      return translate("task:sessionBootstrapCauseUnknown");
  }
}

export function operationLabel(
  operation: string | undefined,
  translate: (key: string) => string,
): string {
  if (operation === "restore_workspace") {
    return translate("task:sessionRecoveryRestoreAttempt");
  }
  return translate("task:sessionRecoveryResumeAttempt");
}

function BootstrapRecoveryActions({
  profileExists,
  busyAction,
  hasBranchRecovery,
  blocked,
  canRestore,
  onResume,
  onRestore,
  onFreshStart,
  onNewBranch,
}: {
  profileExists: boolean;
  busyAction: SessionRecoveryBusyAction;
  hasBranchRecovery: boolean;
  blocked: boolean;
  canRestore: boolean;
  onResume: () => void;
  onRestore: () => void;
  onFreshStart: () => void;
  onNewBranch: () => void;
}) {
  const { t } = useTranslation();
  const actions: RecoveryChoice[] = [
    {
      kind: "resume",
      label: t("task:resume"),
      onClick: onResume,
      disabled: !profileExists,
      testId: "recovery-resume-button",
    },
    {
      kind: "restore",
      label: t("task:restoreReadOnlyWorkspace"),
      onClick: onRestore,
      testId: "recovery-restore-workspace-button",
    },
    {
      kind: "fresh_start",
      label: t("task:startFreshSession"),
      onClick: onFreshStart,
      testId: "recovery-fresh-button",
    },
  ];
  if (hasBranchRecovery)
    actions.push({
      kind: "resume_new_branch",
      label: t("task:continueOnNewBranch"),
      onClick: onNewBranch,
      testId: "recovery-new-branch-button",
    });
  return (
    <RecoveryActions
      actions={canRestore ? actions : actions.filter((action) => action.kind !== "restore")}
      busy={busyAction !== null}
      busyAction={busyAction}
      blocked={blocked}
      preferred={!profileExists ? "fresh_start" : undefined}
    />
  );
}

function automaticRecoveryCauses(
  recovery: SessionRecoveryOwner | null | undefined,
  translate: (key: string) => string,
): AgentErrorCause[] {
  if (!recovery) return [];
  if (recovery.recoveryFailure?.outcome === "recovery_failed") {
    return [
      {
        operation: "resume",
        code: "unknown",
        detail: [
          translate("task:failedToResumeSession"),
          recovery.recoveryFailure.resumeError,
        ].join("\n"),
      },
      {
        operation: "restore_workspace",
        code: "unknown",
        detail: [
          translate("task:failedToRestoreWorkspace"),
          recovery.recoveryFailure.restoreError,
        ].join("\n"),
      },
    ];
  }
  if (recovery.recoveryFailure?.outcome === "workspace_read_only" || recovery.error) {
    return [
      {
        operation: "resume",
        code: "unknown",
        detail: [
          translate("task:failedToResumeSession"),
          recovery.recoveryFailure?.outcome === "workspace_read_only"
            ? recovery.recoveryFailure.resumeError
            : recovery.error,
        ]
          .filter(Boolean)
          .join("\n"),
      },
    ];
  }
  return [];
}

function manualRecoveryCauses(
  failure: ManualSessionRecoveryFailure | null,
  manualError: Error | null,
  translate: (key: string) => string,
): AgentErrorCause[] {
  if (!failure) return [];
  const restore = failure.operation === "restore_workspace";
  return [
    {
      operation: restore ? "restore_workspace" : "resume",
      code: "unknown",
      detail:
        manualError?.message ??
        translate(restore ? "task:failedToRestoreWorkspace" : "task:failedToResumeSession"),
    },
  ];
}

type RecoveryCardModel = {
  causes: AgentErrorCause[];
  displayNotice: string | null;
  hasRecoveryFailure: boolean;
  isReadOnly: boolean;
  hasDetails: boolean;
  titleKey: string;
  summary: string;
};

type RecoveryCardCopy = {
  launchNeedsAttention: string;
  launchErrorNoChanges: string;
  sessionRecoveryDetails: string;
};

function recoveryTitleKey(isReadOnly: boolean, hasRecoveryFailure: boolean): string {
  if (isReadOnly) return "task:resumeFailedWorkspaceReadOnly";
  if (hasRecoveryFailure) return "task:sessionRecoveryFailed";
  return "task:sessionBootstrapRecoveryTitle";
}

function recoverySummary(
  displayNotice: string | null,
  causes: AgentErrorCause[],
  translate: (key: string) => string,
): string {
  if (displayNotice) return displayNotice;
  if (causes.length > 0) return causeLabel(causes[0]?.code, translate);
  return translate("task:sessionBootstrapRecoverySummary");
}

export function buildRecoveryCardModel({
  error,
  automaticRecovery,
  manualFailure,
  manualError,
  recoveryNotice,
  translate,
}: {
  error: TaskStatusSummaryActiveError;
  automaticRecovery?: SessionRecoveryOwner | null;
  manualFailure: ManualSessionRecoveryFailure | null;
  manualError: Error | null;
  recoveryNotice: string | null;
  translate: (key: string) => string;
}): RecoveryCardModel {
  const causes = [
    ...(error.causes ?? []),
    ...automaticRecoveryCauses(automaticRecovery, translate),
    ...manualRecoveryCauses(manualFailure, manualError, translate),
  ];
  const displayNotice = manualFailure
    ? null
    : (recoveryNotice ?? automaticRecovery?.notice ?? null);
  const hasRecoveryFailure =
    automaticRecovery?.recoveryFailure?.outcome === "recovery_failed" || manualFailure !== null;
  const isReadOnly = Boolean(displayNotice) && !hasRecoveryFailure;

  return {
    causes,
    displayNotice,
    hasRecoveryFailure,
    isReadOnly,
    hasDetails: causes.length > 0 || Boolean(error.details),
    titleKey: recoveryTitleKey(isReadOnly, hasRecoveryFailure),
    summary: recoverySummary(displayNotice, causes, translate),
  };
}

function RecoveryCardContent({
  model,
  error,
  profileExists,
  busyAction,
  hasBranchRecovery,
  blocked,
  canRestore,
  onResume,
  onRestore,
  onFreshStart,
  onNewBranch,
  copy,
  translate,
}: {
  model: RecoveryCardModel;
  error: TaskStatusSummaryActiveError;
  profileExists: boolean;
  busyAction: SessionRecoveryBusyAction;
  hasBranchRecovery: boolean;
  blocked: boolean;
  canRestore: boolean;
  onResume: () => void;
  onRestore: () => void;
  onFreshStart: () => void;
  onNewBranch: () => void;
  copy: RecoveryCardCopy;
  translate: (key: string) => string;
}) {
  const profileMissing = translate("task:agentProfileNoLongerExists");
  const details = [
    ...model.causes.map((cause) =>
      [operationLabel(cause.operation, translate), causeLabel(cause.code, translate), cause.detail]
        .filter(Boolean)
        .join("\n"),
    ),
    error.details,
  ]
    .filter(Boolean)
    .join("\n\n");
  return (
    <div className="min-w-0 flex-1">
      <div className="flex min-w-0 flex-wrap items-center gap-2">
        <span className="text-sm font-medium">{translate(model.titleKey)}</span>
        {!model.isReadOnly ? (
          <span className="text-xs text-muted-foreground">{copy.launchNeedsAttention}</span>
        ) : null}
      </div>
      <p className="mt-1 max-w-prose break-words text-sm text-muted-foreground">{model.summary}</p>
      <p className="mt-2 text-xs text-muted-foreground" data-testid="session-bootstrap-no-change">
        {copy.launchErrorNoChanges}
      </p>
      {!profileExists && <p className="mt-1 text-xs text-muted-foreground">{profileMissing}</p>}
      <BootstrapRecoveryActions
        profileExists={profileExists}
        busyAction={busyAction}
        hasBranchRecovery={hasBranchRecovery}
        blocked={blocked}
        canRestore={canRestore}
        onResume={onResume}
        onRestore={onRestore}
        onFreshStart={onFreshStart}
        onNewBranch={onNewBranch}
      />
      {model.hasDetails ? (
        <SessionErrorDetails
          testId="session-bootstrap-recovery-details"
          textTestId="session-bootstrap-cause-details"
          label={copy.sessionRecoveryDetails}
        >
          {details}
        </SessionErrorDetails>
      ) : null}
    </div>
  );
}

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
