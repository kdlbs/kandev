import type { ManualSessionRecoveryFailure } from "@/hooks/domains/session/use-session-recovery-actions";
import type { SessionRecoveryOwner } from "@/lib/session-recovery-presentation";
import type {
  AgentErrorCause,
  TaskStatusSummaryActiveError,
} from "@/lib/types/task-status-summary";

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

export type RecoveryCardModel = {
  causes: AgentErrorCause[];
  displayNotice: string | null;
  hasRecoveryFailure: boolean;
  isReadOnly: boolean;
  hasDetails: boolean;
  titleKey: string;
  summary: string;
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
