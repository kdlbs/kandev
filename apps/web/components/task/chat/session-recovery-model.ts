import { useTranslation } from "react-i18next";
import type { ActiveSessionRecovery } from "@/lib/active-session-recovery";
import type { SessionRecoveryActions } from "@/hooks/domains/session/use-session-recovery-actions";
import type { RecoveryChoice } from "../recovery-actions";
import { sanitizeSessionErrorDetails } from "@/lib/session-error-details";
import { formatDateTime } from "@/lib/i18n/formats";
import type { TaskLaunchErrorContextValue } from "../task-launch-error-context";
import {
  buildRecoveryCardModel,
  automaticRecoveryCauses,
  causeLabel,
  operationLabel,
} from "./session-bootstrap-recovery-model";
import {
  isSessionRecoveryBusy,
  matchingAutomaticRecovery,
  type SessionRecoveryOwner,
} from "@/lib/session-recovery-presentation";
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
  if (model.kind === "managed_clone_relocation_required")
    return {
      title: t("task:managedCloneRelocationTitle"),
      summary: t("task:managedCloneRelocationBody"),
    };
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
  if (kind === "relocate_and_resume")
    return {
      label: t("task:managedCloneRelocateResume"),
      testId: "managed-clone-relocate-button",
    };
  return { label: t("task:resumeSession"), testId: "recovery-resume-button" };
}

export function useRecoveryChoices(
  model: ActiveSessionRecovery,
  actions: SessionRecoveryActions,
  profileExists: boolean,
  onNewSession: () => void,
  onRelocateRequested: () => void,
) {
  const { t } = useTranslation();
  const supplied = model.metadata?.actions
    ?.map(sessionRecoveryAction)
    .filter((kind) => kind !== null);
  const managedCloneRelocation =
    model.kind === "managed_clone_relocation_required" ||
    Boolean(actions.managedCloneRecoveryStamp);
  const kinds = recoveryActionKinds(model, supplied, managedCloneRelocation);
  const choices: RecoveryChoice[] = kinds.map((kind) =>
    createRecoveryChoice({
      kind,
      model,
      actions,
      profileExists,
      onNewSession,
      onRelocateRequested,
      t,
    }),
  );
  if (
    !managedCloneRelocation &&
    !isManagedRuntimeFailure(model.kind) &&
    (actions.recoveryError || isBootstrapRecovery(model))
  )
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

function recoveryActionKinds(
  model: ActiveSessionRecovery,
  supplied: NonNullable<ReturnType<typeof sessionRecoveryAction>>[] | undefined,
  managedCloneRelocation: boolean,
) {
  if (managedCloneRelocation) return ["relocate_and_resume"] as const;
  if (model.kind === "provider_quota_limited" && !supplied) return ["resume"] as const;
  if (isManagedRuntimeFailure(model.kind)) return ["runtime_retry"] as const;
  if (model.kind === "missing_pr_branch" && !supplied) return [] as const;
  return supplied ?? (["resume", "fresh_start"] as const);
}

function createRecoveryChoice({
  kind,
  model,
  actions,
  profileExists,
  onNewSession,
  onRelocateRequested,
  t,
}: {
  kind: NonNullable<ReturnType<typeof sessionRecoveryAction>>;
  model: ActiveSessionRecovery;
  actions: SessionRecoveryActions;
  profileExists: boolean;
  onNewSession: () => void;
  onRelocateRequested: () => void;
  t: ReturnType<typeof useTranslation>["t"];
}): RecoveryChoice {
  const copy = recoveryActionCopy(kind, t);
  return {
    kind,
    label: copy.label,
    testId: copy.testId,
    disabled: kind === "resume" && !profileExists,
    tooltip: model.metadata?.actions?.find((action) => sessionRecoveryAction(action) === kind)
      ?.tooltip,
    onClick: () => {
      if (kind === "fresh_start" && !profileExists) {
        onNewSession();
        return;
      }
      if (kind === "relocate_and_resume") {
        onRelocateRequested();
        return;
      }
      void actions.handleRecover(kind);
    },
  };
}

export function useRecoveryPresentation(
  model: ActiveSessionRecovery,
  actions: SessionRecoveryActions,
  context: TaskLaunchErrorContextValue | null,
) {
  const { t } = useTranslation();
  const automatic = matchingAutomaticRecovery(
    context?.automaticRecovery,
    context?.taskId,
    model.sessionId,
  );
  const managedCloneRelocation =
    model.kind === "managed_clone_relocation_required" ||
    Boolean(actions.managedCloneRecoveryStamp);
  const bootstrap = managedCloneRelocation
    ? null
    : buildBootstrapRecoveryModel(model, actions, automatic, t);
  const copy = recoveryPresentationCopy(model, bootstrap, managedCloneRelocation, t);
  const busy =
    Boolean(model.loading) ||
    isSessionRecoveryBusy(automatic?.resumptionState ?? "idle") ||
    actions.busyAction !== null;
  const busyAction = automatic?.resumptionState === "resuming" ? "resume" : actions.busyAction;
  const causes = bootstrap?.causes ?? automaticRecoveryCauses(automatic, t);
  const details = recoveryPresentationDetails(model, causes, actions, t);
  const failure = recoveryFailureCopy(actions, t);
  return {
    copy: withAutomaticNotice(copy, automatic, actions),
    busy,
    busyAction,
    details,
    failure,
  };
}

function withAutomaticNotice(
  copy: ReturnType<typeof recoveryPresentationCopy>,
  automatic: SessionRecoveryOwner | null,
  actions: SessionRecoveryActions,
) {
  return automatic?.notice && !actions.recoveryError
    ? { ...copy, summary: automatic.notice }
    : copy;
}

function buildBootstrapRecoveryModel(
  model: ActiveSessionRecovery,
  actions: SessionRecoveryActions,
  automatic: SessionRecoveryOwner | null | undefined,
  t: ReturnType<typeof useTranslation>["t"],
) {
  if (!isBootstrapRecovery(model)) return null;
  return buildRecoveryCardModel({
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
  });
}

function recoveryPresentationCopy(
  model: ActiveSessionRecovery,
  bootstrap: ReturnType<typeof buildRecoveryCardModel> | null,
  managedCloneRelocation: boolean,
  t: ReturnType<typeof useTranslation>["t"],
) {
  if (managedCloneRelocation)
    return recoveryCopy({ ...model, kind: "managed_clone_relocation_required" }, t);
  if (bootstrap) return { title: t(bootstrap.titleKey), summary: bootstrap.summary };
  return recoveryCopy(model, t);
}

function recoveryPresentationDetails(
  model: ActiveSessionRecovery,
  causes: NonNullable<ReturnType<typeof buildRecoveryCardModel>>["causes"],
  actions: SessionRecoveryActions,
  t: ReturnType<typeof useTranslation>["t"],
) {
  return [
    model.details,
    ...causes.map((cause) =>
      [operationLabel(cause.operation, t), causeLabel(cause.code, t), cause.detail]
        .filter(Boolean)
        .join("\n"),
    ),
    actions.recoveryError?.message,
  ]
    .filter(Boolean)
    .join("\n\n");
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
    model.kind !== "provider_quota_limited" &&
    model.kind !== "managed_clone_relocation_required"
  );
}

function isManagedRuntimeFailure(kind: string) {
  return kind === "managed_runtime_npm_resolution" || kind === "managed_runtime_npm_policy";
}
