import type { TFunction } from "i18next";
import type { ManualSessionRecoveryFailure } from "@/hooks/domains/session/use-session-recovery-actions";
import type { SessionRecoveryOwner } from "@/lib/session-recovery-presentation";
import type { SessionErrorDetailsField } from "@/lib/session-error-details";
import { normalizeAgentErrorCauses } from "@/lib/session-last-agent-error";
import { sanitizeSessionErrorDetails } from "@/lib/session-error-details";
import type {
  AgentErrorCause,
  TaskStatusSummaryActiveError,
} from "@/lib/types/task-status-summary";

const selectionCauseCodes = new Set([
  "model_unavailable",
  "model_selection_failed",
  "permission_mode_failed",
  "permission_mode_unconfirmed",
  "permission_mode_mismatch",
]);

export function causeLabel(code: string | undefined, translate: TFunction): string {
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
    case "model_unavailable":
      return translate("task:sessionBootstrapCauseModelUnavailable");
    case "model_selection_failed":
      return translate("task:sessionBootstrapCauseModelSelectionFailed");
    case "permission_mode_failed":
      return translate("task:sessionBootstrapCausePermissionModeFailed");
    case "permission_mode_unconfirmed":
      return translate("task:sessionBootstrapCausePermissionModeUnconfirmed");
    case "permission_mode_mismatch":
      return translate("task:sessionBootstrapCausePermissionModeMismatch");
    default:
      return translate("task:sessionBootstrapCauseUnknown");
  }
}

export function operationLabel(operation: string | undefined, translate: TFunction): string {
  if (operation === "start") return translate("task:sessionRecoveryStartAttempt");
  if (operation === "restore_workspace") {
    return translate("task:sessionRecoveryRestoreAttempt");
  }
  return translate("task:sessionRecoveryResumeAttempt");
}

export function automaticRecoveryCauses(
  recovery: SessionRecoveryOwner | null | undefined,
  translate: TFunction,
): AgentErrorCause[] {
  if (!recovery) return [];
  if (recovery.recoveryFailure?.outcome === "recovery_failed") {
    return [
      {
        operation: "resume",
        code: "unknown",
        detail: [translate("task:failedToResumeSession"), recovery.recoveryFailure.resumeError]
          .filter(Boolean)
          .join("\n"),
      },
      {
        operation: "restore_workspace",
        code: "unknown",
        detail: [translate("task:failedToRestoreWorkspace"), recovery.recoveryFailure.restoreError]
          .filter(Boolean)
          .join("\n"),
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
  error: TaskStatusSummaryActiveError,
  translate: TFunction,
): AgentErrorCause[] {
  if (!failure || failure.sessionId !== error.session_id || failure.errorStamp !== error.stamp) {
    return [];
  }
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
  primaryCause: AgentErrorCause | null;
  displayNotice: string | null;
  workspaceStatus: string | null;
  hasRecoveryFailure: boolean;
  isReadOnly: boolean;
  showSummary: boolean;
  hasDetails: boolean;
  noPromptSent: boolean;
  freshStartWarning: boolean;
  detailFields: SessionErrorDetailsField[];
  hasTypedSelectionCause: boolean;
  titleKey: string;
  summary: string;
};

type RecoveryTranslationOptions = {
  agent?: string;
  model?: string;
  mode?: string;
  effectiveMode?: string;
};

function recoveryTitleKey(isReadOnly: boolean, hasRecoveryFailure: boolean): string {
  if (isReadOnly) return "task:resumeFailedWorkspaceReadOnly";
  if (hasRecoveryFailure) return "task:sessionRecoveryFailed";
  return "task:sessionBootstrapRecoveryTitle";
}

function selectionTitleKey(cause: AgentErrorCause): string | null {
  switch (cause.code) {
    case "model_unavailable":
      return "task:sessionBootstrapModelUnavailableTitle";
    case "model_selection_failed":
      switch (cause.reason) {
        case "catalog_empty":
          return "task:sessionBootstrapModelCatalogEmptyTitle";
        case "selection_unsupported":
          return "task:sessionBootstrapModelSelectionUnsupportedTitle";
        case "application_failed":
          return "task:sessionBootstrapModelSelectionFailedTitle";
        case "selection_missing":
          return "task:sessionBootstrapModelRequiredTitle";
        default:
          return null;
      }
    case "permission_mode_unconfirmed":
      return "task:sessionBootstrapModeUnconfirmedTitle";
    case "permission_mode_mismatch":
      return "task:sessionBootstrapModeMismatchTitle";
    case "permission_mode_failed":
      return "task:sessionBootstrapModeFailedTitle";
    default:
      return null;
  }
}

function modelSelectionSummaryKey(cause: AgentErrorCause): string | null {
  switch (cause.code) {
    case "model_unavailable":
      return cause.requested_model
        ? "task:sessionBootstrapModelUnavailableBody"
        : "task:sessionBootstrapModelUnavailableBodyUnknown";
    case "model_selection_failed":
      switch (cause.reason) {
        case "catalog_empty":
          return "task:sessionBootstrapModelCatalogEmptyBody";
        case "selection_unsupported":
          return "task:sessionBootstrapModelSelectionUnsupportedBody";
        case "application_failed":
          return cause.attempted_model
            ? "task:sessionBootstrapModelSelectionFailedBody"
            : "task:sessionBootstrapModelSelectionFailedBodyUnknown";
        case "selection_missing":
          return "task:sessionBootstrapModelRequiredBody";
        default:
          return null;
      }
    default:
      return null;
  }
}

function modeSelectionSummaryKey(cause: AgentErrorCause): string | null {
  switch (cause.code) {
    case "permission_mode_unconfirmed":
      return cause.requested_mode
        ? "task:sessionBootstrapModeUnconfirmedBody"
        : "task:sessionBootstrapModeUnconfirmedBodyUnknown";
    case "permission_mode_mismatch":
      return cause.requested_mode && cause.effective_mode
        ? "task:sessionBootstrapModeMismatchBody"
        : "task:sessionBootstrapModeMismatchBodyUnknown";
    case "permission_mode_failed":
      if (!cause.requested_mode) return "task:sessionBootstrapModeFailedBodyUnknown";
      if (cause.reason === "client_unavailable")
        return "task:sessionBootstrapModeClientUnavailableBody";
      return "task:sessionBootstrapModeFailedBody";
    default:
      return null;
  }
}

function selectionSummaryKey(cause: AgentErrorCause): string | null {
  return modelSelectionSummaryKey(cause) ?? modeSelectionSummaryKey(cause);
}

function selectorSummaryOptions(
  cause: AgentErrorCause,
  agentDisplayName: string | undefined,
  translate: TFunction,
): RecoveryTranslationOptions {
  const candidate = agentDisplayName ? sanitizeSessionErrorDetails(agentDisplayName, 80) : "";
  const safeAgent =
    candidate && candidate === agentDisplayName
      ? candidate
      : translate("task:sessionBootstrapAgentFallback");
  const model =
    cause.code === "model_selection_failed" && cause.reason === "application_failed"
      ? cause.attempted_model
      : cause.requested_model;
  return {
    agent: safeAgent,
    ...(model ? { model } : {}),
    ...(cause.requested_mode ? { mode: cause.requested_mode } : {}),
    ...(cause.effective_mode ? { effectiveMode: cause.effective_mode } : {}),
  };
}

function causeSummary(
  cause: AgentErrorCause | null,
  agentDisplayName: string | undefined,
  translate: TFunction,
): string {
  if (!cause) return translate("task:sessionBootstrapRecoverySummary");
  const key = isStartupSelectionCause(cause) ? selectionSummaryKey(cause) : null;
  if (key) return translate(key, selectorSummaryOptions(cause, agentDisplayName, translate));
  return causeLabel(cause.code, translate);
}

function isStartupSelectionCause(cause: AgentErrorCause): boolean {
  return (
    (cause.operation === "start" || cause.operation === "resume") &&
    selectionCauseCodes.has(cause.code ?? "")
  );
}

function selectPrimaryCause(causes: AgentErrorCause[]): AgentErrorCause | null {
  return (
    causes.find(isStartupSelectionCause) ??
    causes.find((cause) => cause.operation !== "restore_workspace") ??
    causes[0] ??
    null
  );
}

function recoveryDetailFields(
  error: TaskStatusSummaryActiveError,
  causes: AgentErrorCause[],
  translate: TFunction,
): SessionErrorDetailsField[] {
  const fields: SessionErrorDetailsField[] = [];
  if (error.phase) {
    appendDetailField(fields, translate("task:sessionErrorDetailPhase"), error.phase);
  }
  for (const cause of causes) {
    appendDetailField(
      fields,
      translate("task:sessionErrorDetailOperation"),
      operationLabel(cause.operation, translate),
    );
    if (cause.code)
      appendDetailField(fields, translate("task:sessionErrorDetailCause"), cause.code);
    if (cause.reason)
      appendDetailField(fields, translate("task:sessionErrorDetailReason"), cause.reason);
    appendSelectorDetail(
      fields,
      "task:sessionErrorDetailRequestedModel",
      cause.requested_model,
      translate,
    );
    appendSelectorDetail(
      fields,
      "task:sessionErrorDetailAttemptedModel",
      cause.attempted_model,
      translate,
    );
    appendSelectorDetail(
      fields,
      "task:sessionErrorDetailEffectiveModel",
      cause.effective_model,
      translate,
    );
    appendSelectorDetail(
      fields,
      "task:sessionErrorDetailRequestedMode",
      cause.requested_mode,
      translate,
    );
    appendSelectorDetail(
      fields,
      "task:sessionErrorDetailEffectiveMode",
      cause.effective_mode,
      translate,
    );
    if (typeof cause.prompt_not_sent === "boolean") {
      appendDetailField(
        fields,
        translate("task:sessionErrorDetailPromptSent"),
        translate(
          cause.prompt_not_sent ? "task:sessionErrorPromptNotSent" : "task:sessionErrorPromptSent",
        ),
      );
    }
    if (cause.detail)
      appendDetailField(fields, translate("task:sessionErrorDetailMessage"), cause.detail);
  }
  if (error.occurred_at && !Number.isNaN(Date.parse(error.occurred_at))) {
    appendDetailField(fields, translate("task:sessionErrorDetailOccurred"), error.occurred_at);
  }
  if (error.attempt_id) {
    fields.push({
      label: translate("task:sessionErrorDetailAttempt"),
      value: error.attempt_id,
      kind: "host-attempt-reference",
    });
  }
  if (error.execution_id) {
    fields.push({
      label: translate("task:sessionErrorDetailExecution"),
      value: error.execution_id,
      kind: "host-execution-reference",
    });
  }
  return fields;
}

function appendSelectorDetail(
  fields: SessionErrorDetailsField[],
  key: string,
  value: string | undefined,
  translate: TFunction,
) {
  if (value) appendDetailField(fields, translate(key), value);
}

function appendDetailField(fields: SessionErrorDetailsField[], label: string, value: string) {
  fields.push({ label, value });
}

type RecoveryCardStatus = Pick<
  RecoveryCardModel,
  "displayNotice" | "hasRecoveryFailure" | "isReadOnly" | "workspaceStatus"
>;

function currentManualRecoveryFailure(
  error: TaskStatusSummaryActiveError,
  manualFailure: ManualSessionRecoveryFailure | null,
): boolean {
  return Boolean(
    manualFailure &&
    manualFailure.sessionId === error.session_id &&
    manualFailure.errorStamp === error.stamp,
  );
}

function readOnlyWorkspaceOutcome(
  automaticRecovery: SessionRecoveryOwner | null | undefined,
  recoveryNotice: string | null,
): boolean {
  return (
    automaticRecovery?.recoveryFailure?.outcome === "workspace_read_only" ||
    Boolean(recoveryNotice ?? automaticRecovery?.notice)
  );
}

function anyRecoveryFailure(
  automaticRecovery: SessionRecoveryOwner | null | undefined,
  manualFailureExists: boolean,
): boolean {
  return automaticRecovery?.recoveryFailure?.outcome === "recovery_failed" || manualFailureExists;
}

function displayReadOnlyWorkspace(
  automaticRecovery: SessionRecoveryOwner | null | undefined,
  readOnlyOutcome: boolean,
  hasRecoveryFailure: boolean,
): boolean {
  return (
    readOnlyOutcome &&
    !hasRecoveryFailure &&
    automaticRecovery?.recoveryFailure?.outcome !== "status_unavailable"
  );
}

function recoveryCardStatus({
  error,
  automaticRecovery,
  manualFailure,
  recoveryNotice,
  primaryCause,
  translate,
}: {
  error: TaskStatusSummaryActiveError;
  automaticRecovery: SessionRecoveryOwner | null | undefined;
  manualFailure: ManualSessionRecoveryFailure | null;
  recoveryNotice: string | null;
  primaryCause: AgentErrorCause | null;
  translate: TFunction;
}): RecoveryCardStatus {
  const displayNotice = recoveryNotice ?? automaticRecovery?.notice ?? null;
  const readOnlyOutcome = readOnlyWorkspaceOutcome(automaticRecovery, recoveryNotice);
  const hasRecoveryFailure = anyRecoveryFailure(
    automaticRecovery,
    currentManualRecoveryFailure(error, manualFailure),
  );
  const isReadOnly = displayReadOnlyWorkspace(
    automaticRecovery,
    readOnlyOutcome,
    hasRecoveryFailure,
  );

  return {
    displayNotice,
    hasRecoveryFailure,
    isReadOnly,
    workspaceStatus:
      isReadOnly && primaryCause ? translate("task:sessionRecoveryWorkspaceReadOnly") : null,
  };
}

export function buildRecoveryCardModel({
  error,
  automaticRecovery,
  manualFailure,
  manualError,
  recoveryNotice,
  translate,
  agentDisplayName,
}: {
  error: TaskStatusSummaryActiveError;
  automaticRecovery?: SessionRecoveryOwner | null;
  manualFailure: ManualSessionRecoveryFailure | null;
  manualError: Error | null;
  recoveryNotice: string | null;
  translate: TFunction;
  agentDisplayName?: string;
}): RecoveryCardModel {
  const causes = [
    ...normalizeAgentErrorCauses(error.causes),
    ...automaticRecoveryCauses(automaticRecovery, translate),
    ...manualRecoveryCauses(manualFailure, manualError, error, translate),
  ];
  const primaryCause = selectPrimaryCause(causes);
  const status = recoveryCardStatus({
    error,
    automaticRecovery,
    manualFailure,
    recoveryNotice,
    primaryCause,
    translate,
  });
  const hasTypedSelectionCause = causes.some(isStartupSelectionCause);
  const causeTitleKey =
    primaryCause && isStartupSelectionCause(primaryCause) ? selectionTitleKey(primaryCause) : null;
  const titleKey = causeTitleKey ?? recoveryTitleKey(status.isReadOnly, status.hasRecoveryFailure);

  return {
    causes,
    primaryCause,
    ...status,
    showSummary: !status.isReadOnly || Boolean(primaryCause),
    hasDetails: causes.length > 0 || Boolean(error.details),
    noPromptSent: Boolean(
      primaryCause && isStartupSelectionCause(primaryCause) && primaryCause.prompt_not_sent,
    ),
    freshStartWarning: Boolean(primaryCause && isStartupSelectionCause(primaryCause)),
    detailFields: recoveryDetailFields(error, causes, translate),
    hasTypedSelectionCause,
    titleKey,
    summary: causeSummary(primaryCause, agentDisplayName, translate),
  };
}
