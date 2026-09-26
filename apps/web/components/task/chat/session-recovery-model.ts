import { useTranslation } from "react-i18next";
import type { ActiveSessionRecovery } from "@/lib/active-session-recovery";
import type { SessionRecoveryActions } from "@/hooks/domains/session/use-session-recovery-actions";
import type { RecoveryChoice } from "../recovery-actions";
import { sanitizeSessionErrorDetails } from "@/lib/session-error-details";
import { formatDateTime } from "@/lib/i18n/formats";
import type { TaskLaunchErrorContextValue } from "../task-launch-error-context";
import {
  buildRecoveryCardModel,
  causeLabel,
  operationLabel,
} from "./session-bootstrap-recovery-card";
import { isSessionRecoveryBusy } from "@/lib/session-recovery-presentation";
import { sessionRecoveryAction } from "./messages/action-message-recovery";

function recoveryCopy(model: ActiveSessionRecovery, t: ReturnType<typeof useTranslation>["t"]) {
  if (model.kind === "managed_runtime_npm_resolution")
    return { title: t("chat:managedRuntimeNpmTitle"), summary: t("chat:managedRuntimeNpmBody") };
  if (model.kind === "managed_runtime_npm_policy")
    return {
      title: t("chat:managedRuntimeNpmPolicyTitle"),
      summary: t("chat:managedRuntimeNpmPolicyBody"),
    };
  if (model.kind === "provider_quota_limited") {
    const reset = model.metadata?.reset_at ? new Date(model.metadata.reset_at) : null;
    return {
      title: t("chat:providerQuotaTitle", {
        provider: model.metadata?.provider_name || t("chat:providerQuotaProviderFallback"),
      }),
      summary:
        reset && !Number.isNaN(reset.getTime())
          ? t("chat:providerQuotaReset", { resetAt: formatDateTime(reset) })
          : t("chat:providerQuotaResetUnknown"),
    };
  }
  const summary = model.summary?.trim();
  const safe = summary && summary.length <= 240 && sanitizeSessionErrorDetails(summary) === summary;
  return {
    title: t("task:sessionRecoveryFailed"),
    summary: safe ? summary : t("task:agentHasStopped"),
  };
}

function recoveryActionCopy(kind: string, t: ReturnType<typeof useTranslation>["t"]) {
  if (kind === "runtime_retry")
    return { label: t("chat:managedRuntimeRetry"), testId: "managed-runtime-npm-retry-button" };
  if (kind === "fresh_start")
    return { label: t("task:startFreshSession"), testId: "recovery-fresh-button" };
  if (kind === "resume_new_branch")
    return { label: t("task:continueOnNewBranch"), testId: "recovery-new-branch-button" };
  return { label: t("task:resumeSession"), testId: "recovery-resume-button" };
}

export function useRecoveryChoices(
  model: ActiveSessionRecovery,
  actions: SessionRecoveryActions,
  profileExists: boolean,
  onNewSession: () => void,
) {
  const { t } = useTranslation();
  const supplied = model.metadata?.actions
    ?.map(sessionRecoveryAction)
    .filter((kind) => kind !== null);
  let kinds = supplied ?? ["resume" as const, "fresh_start" as const];
  if (model.kind === "provider_quota_limited" && !supplied) kinds = ["resume"];
  if (isManagedRuntimeFailure(model.kind)) kinds = ["runtime_retry"];
  if (model.kind === "missing_pr_branch" && !supplied) kinds = [];
  const choices: RecoveryChoice[] = kinds.map((kind) => ({
    kind,
    label: recoveryActionCopy(kind, t).label,
    testId: recoveryActionCopy(kind, t).testId,
    disabled: kind === "resume" && !profileExists,
    tooltip: model.metadata?.actions?.find((action) => sessionRecoveryAction(action) === kind)
      ?.tooltip,
    onClick: () => {
      if (kind === "fresh_start" && !profileExists) onNewSession();
      else void actions.handleRecover(kind);
    },
  }));
  if (!isManagedRuntimeFailure(model.kind) && (actions.recoveryError || isBootstrapRecovery(model)))
    choices.push({
      kind: "restore",
      label: t("task:restoreReadOnlyWorkspace"),
      testId: "recovery-restore-workspace-button",
      onClick: () => void actions.handleRestore(),
    });
  if (actions.branchDetails && !choices.some((action) => action.kind === "resume_new_branch"))
    choices.push({
      kind: "resume_new_branch",
      label: t("task:continueOnNewBranch"),
      testId: "recovery-new-branch-button",
      onClick: () => void actions.handleNewBranch(),
    });
  return actions.guardDetails ? choices.filter((choice) => choice.kind !== "restore") : choices;
}

export function useRecoveryPresentation(
  model: ActiveSessionRecovery,
  actions: SessionRecoveryActions,
  context: TaskLaunchErrorContextValue | null,
) {
  const { t } = useTranslation();
  const automatic = matchingAutomaticRecovery(context, model.sessionId);
  const bootstrap = isBootstrapRecovery(model)
    ? buildRecoveryCardModel({
        error: {
          stamp: model.stamp ?? "",
          occurred_at: "",
          preview: model.summary ?? "",
          details: model.details,
          causes: model.error?.causes,
        },
        automaticRecovery: automatic,
        manualFailure: actions.manualRecoveryFailure,
        manualError: actions.recoveryError,
        recoveryNotice: actions.recoveryNotice,
        translate: t,
      })
    : null;
  const copy = bootstrap
    ? { title: t(bootstrap.titleKey), summary: bootstrap.summary }
    : recoveryCopy(model, t);
  const busy =
    Boolean(model.loading) ||
    isSessionRecoveryBusy(automatic?.resumptionState ?? "idle") ||
    actions.busyAction !== null;
  const busyAction = automatic?.resumptionState === "resuming" ? "resume" : actions.busyAction;
  const details = [
    model.details,
    ...(bootstrap?.causes ?? []).map((cause) =>
      [operationLabel(cause.operation, t), causeLabel(cause.code, t), cause.detail]
        .filter(Boolean)
        .join("\n"),
    ),
    actions.recoveryError?.message,
  ]
    .filter(Boolean)
    .join("\n\n");
  const failure = recoveryFailureCopy(actions, t);
  return { copy, busy, busyAction, details, failure };
}

function recoveryFailureCopy(
  actions: SessionRecoveryActions,
  t: ReturnType<typeof useTranslation>["t"],
) {
  let failure =
    actions.manualRecoveryFailure?.operation === "restore_workspace"
      ? t("task:failedToRestoreWorkspace")
      : t("task:failedToResumeSession");
  if (actions.branchDetails) failure = t("task:branchIsNoLongerAvailable");
  if (actions.guardDetails && actions.recoveryError)
    failure = sanitizeSessionErrorDetails(actions.recoveryError.message, 240) || failure;
  return failure;
}

function isBootstrapRecovery(model: ActiveSessionRecovery) {
  return (
    model.error?.phase === "bootstrap" &&
    !isManagedRuntimeFailure(model.kind) &&
    model.kind !== "provider_quota_limited"
  );
}

function matchingAutomaticRecovery(context: TaskLaunchErrorContextValue | null, sessionId: string) {
  return context?.statusSummary?.active_error?.session_id === sessionId
    ? context.automaticRecovery
    : null;
}

function isManagedRuntimeFailure(kind: string) {
  return kind === "managed_runtime_npm_resolution" || kind === "managed_runtime_npm_policy";
}
