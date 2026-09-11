"use client";

import { useCallback, useState } from "react";
import {
  IconAlertTriangle,
  IconInfoCircle,
  IconLoader2,
  IconPlayerPlay,
  IconRefresh,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";
import { NewSessionDialog } from "@/components/task/new-session-dialog";
import {
  type ManualSessionRecoveryFailure,
  useSessionRecoveryActions,
  type SessionRecoveryBusyAction,
} from "@/hooks/domains/session/use-session-recovery-actions";
import {
  isSessionRecoveryBusy,
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

function causeLabel(code: string | undefined, translate: (key: string) => string): string {
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

function operationLabel(operation: string | undefined, translate: (key: string) => string): string {
  if (operation === "restore_workspace") {
    return translate("task:sessionRecoveryRestoreAttempt");
  }
  return translate("task:sessionRecoveryResumeAttempt");
}

function CauseDetails({
  causes,
  translate,
}: {
  causes: AgentErrorCause[];
  translate: (key: string) => string;
}) {
  return (
    <dl className="mt-2 grid min-w-0 gap-2 text-xs" data-testid="session-bootstrap-cause-details">
      {causes.map((cause, index) => (
        <div
          className="min-w-0"
          key={`${cause.operation ?? "cause"}-${cause.code ?? "unknown"}-${index}`}
        >
          <dt className="font-medium">
            {operationLabel(cause.operation, translate)}: {causeLabel(cause.code, translate)}
          </dt>
          {cause.detail ? (
            <dd className="mt-0.5 break-words text-muted-foreground">{cause.detail}</dd>
          ) : null}
        </div>
      ))}
    </dl>
  );
}

function BootstrapRecoveryActions({
  profileExists,
  busyAction,
  hasBranchRecovery,
  onResume,
  onRestore,
  onFreshStart,
  onNewBranch,
}: {
  profileExists: boolean;
  busyAction: SessionRecoveryBusyAction;
  hasBranchRecovery: boolean;
  onResume: () => void;
  onRestore: () => void;
  onFreshStart: () => void;
  onNewBranch: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="mt-3 flex min-w-0 flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center">
      <Tooltip>
        <TooltipTrigger asChild>
          <span
            className="inline-flex w-full sm:w-auto"
            data-testid="bootstrap-recovery-resume-wrapper"
            tabIndex={busyAction !== null || !profileExists ? 0 : -1}
          >
            <Button
              variant="default"
              className="h-auto min-h-11 w-full cursor-pointer justify-start gap-1.5 text-xs sm:w-auto sm:min-h-7"
              onClick={onResume}
              disabled={busyAction !== null || !profileExists}
              data-testid="recovery-resume-button"
            >
              {busyAction === "resume" ? (
                <IconLoader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <IconPlayerPlay className="h-3.5 w-3.5" />
              )}
              {busyAction === "resume" ? t("task:resuming") : t("task:resume")}
            </Button>
          </span>
        </TooltipTrigger>
        {!profileExists ? (
          <TooltipContent>{t("task:agentProfileNoLongerExists")}</TooltipContent>
        ) : null}
      </Tooltip>
      <Button
        variant="outline"
        className="h-auto min-h-11 w-full cursor-pointer justify-start gap-1.5 text-xs sm:w-auto sm:min-h-7"
        onClick={onRestore}
        disabled={busyAction !== null}
        data-testid="recovery-restore-workspace-button"
      >
        {busyAction === "restore" ? (
          <IconLoader2 className="h-3.5 w-3.5 animate-spin" />
        ) : (
          <IconRefresh className="h-3.5 w-3.5" />
        )}
        {t("task:restoreReadOnlyWorkspace")}
      </Button>
      <Button
        variant="outline"
        className="h-auto min-h-11 w-full cursor-pointer justify-start gap-1.5 text-xs sm:w-auto sm:min-h-7"
        onClick={onFreshStart}
        disabled={busyAction !== null}
        data-testid="recovery-fresh-button"
      >
        <IconRefresh className="h-3.5 w-3.5" />
        {busyAction === "fresh_start" ? t("task:starting") : t("task:startFreshSession")}
      </Button>
      {hasBranchRecovery ? (
        <Button
          variant="outline"
          className="h-auto min-h-11 w-full cursor-pointer justify-start gap-1.5 text-xs sm:w-auto sm:min-h-7"
          onClick={onNewBranch}
          disabled={busyAction !== null}
          data-testid="recovery-new-branch-button"
        >
          <IconRefresh className="h-3.5 w-3.5" />
          {t("task:continueOnNewBranch")}
        </Button>
      ) : null}
    </div>
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
        detail: translate("task:failedToResumeSession"),
      },
      {
        operation: "restore_workspace",
        code: "unknown",
        detail: translate("task:failedToRestoreWorkspace"),
      },
    ];
  }
  if (recovery.recoveryFailure?.outcome === "workspace_read_only" || recovery.error) {
    return [
      {
        operation: "resume",
        code: "unknown",
        detail: translate("task:failedToResumeSession"),
      },
    ];
  }
  return [];
}

function manualRecoveryCauses(
  failure: ManualSessionRecoveryFailure | null,
  translate: (key: string) => string,
): AgentErrorCause[] {
  if (!failure) return [];
  const restore = failure.operation === "restore_workspace";
  return [
    {
      operation: restore ? "restore_workspace" : "resume",
      code: "unknown",
      detail: translate(restore ? "task:failedToRestoreWorkspace" : "task:failedToResumeSession"),
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

function buildRecoveryCardModel({
  error,
  automaticRecovery,
  manualFailure,
  recoveryNotice,
  translate,
}: {
  error: TaskStatusSummaryActiveError;
  automaticRecovery?: SessionRecoveryOwner | null;
  manualFailure: ManualSessionRecoveryFailure | null;
  recoveryNotice: string | null;
  translate: (key: string) => string;
}): RecoveryCardModel {
  const causes = [
    ...(error.causes ?? []),
    ...automaticRecoveryCauses(automaticRecovery, translate),
    ...manualRecoveryCauses(manualFailure, translate),
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
  showDetails,
  onDetailsToggle,
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
  showDetails: boolean;
  onDetailsToggle: (open: boolean) => void;
  onResume: () => void;
  onRestore: () => void;
  onFreshStart: () => void;
  onNewBranch: () => void;
  copy: RecoveryCardCopy;
  translate: (key: string) => string;
}) {
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
      {model.hasDetails ? (
        <details
          className="mt-2 min-w-0 text-xs"
          open={showDetails}
          onToggle={(event) => onDetailsToggle(event.currentTarget.open)}
          data-testid="session-bootstrap-recovery-details"
        >
          <summary className="block min-h-11 max-w-full cursor-pointer select-none rounded-sm py-3 underline underline-offset-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:min-h-7">
            {copy.sessionRecoveryDetails}
          </summary>
          <CauseDetails causes={model.causes} translate={translate} />
          {error.details ? (
            <pre className="mt-2 max-w-prose whitespace-pre-wrap break-words rounded bg-muted/50 p-2 font-mono text-[11px] leading-relaxed text-muted-foreground">
              {error.details}
            </pre>
          ) : null}
        </details>
      ) : null}
      <BootstrapRecoveryActions
        profileExists={profileExists}
        busyAction={busyAction}
        hasBranchRecovery={hasBranchRecovery}
        onResume={onResume}
        onRestore={onRestore}
        onFreshStart={onFreshStart}
        onNewBranch={onNewBranch}
      />
    </div>
  );
}

export function SessionBootstrapRecoveryCard({
  taskId,
  sessionId,
  workspaceId,
  error,
  automaticRecovery,
}: SessionBootstrapRecoveryCardProps) {
  const { t } = useTranslation();
  const [showDialog, setShowDialog] = useState(false);
  const [showDetails, setShowDetails] = useState(false);
  const {
    busyAction,
    recoveryError,
    manualRecoveryFailure,
    branchDetails,
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
    recoveryNotice,
    translate: t,
  });
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
      role={model.isReadOnly ? "status" : "alert"}
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
        showDetails={showDetails}
        onDetailsToggle={setShowDetails}
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
