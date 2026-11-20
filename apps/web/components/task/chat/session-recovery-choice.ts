import type { TFunction } from "i18next";
import type { ActiveSessionRecovery } from "@/lib/active-session-recovery";
import type { SessionRecoveryActions } from "@/hooks/domains/session/use-session-recovery-actions";
import type { RecoveryChoice } from "../recovery-actions";
import { sessionRecoveryAction } from "./messages/action-message-recovery";

function recoveryActionCopy(kind: string, t: TFunction, authenticationRequired: boolean) {
  if (kind === "runtime_retry")
    return { label: t("chat:managedRuntimeRetry"), testId: "managed-runtime-npm-retry-button" };
  if (kind === "fresh_start")
    return {
      label: t(authenticationRequired ? "task:restartSession" : "task:startFreshSession"),
      testId: "recovery-fresh-button",
    };
  if (kind === "resume_new_branch")
    return { label: t("task:continueOnNewBranch"), testId: "recovery-new-branch-button" };
  if (kind === "relocate_and_resume")
    return {
      label: t("task:managedCloneRelocateResume"),
      testId: "managed-clone-relocate-button",
    };
  return { label: t("task:resumeSession"), testId: "recovery-resume-button" };
}

export function createRecoveryChoice({
  kind,
  model,
  actions,
  profileExists,
  onNewSession,
  onRelocateRequested,
  onInspectionRetry,
  inspectionBusy,
  authenticationRequired,
  t,
}: {
  kind: NonNullable<ReturnType<typeof sessionRecoveryAction>>;
  model: ActiveSessionRecovery;
  actions: SessionRecoveryActions;
  profileExists: boolean;
  onNewSession: () => void;
  onRelocateRequested: () => void;
  onInspectionRetry?: () => Promise<boolean> | undefined;
  inspectionBusy: boolean;
  authenticationRequired: boolean;
  t: TFunction;
}): RecoveryChoice {
  const copy = recoveryActionCopy(kind, t, authenticationRequired);
  return {
    kind,
    label: copy.label,
    testId: copy.testId,
    disclosure:
      kind === "resume" && actions.providerRestoredResumeEligible
        ? t("task:providerRestoredResumeDisclosure")
        : undefined,
    disabled: kind === "resume" && !profileExists,
    tooltip: model.metadata?.actions?.find((action) => sessionRecoveryAction(action) === kind)
      ?.tooltip,
    onClick: () => {
      if (inspectionBusy && kind === "resume" && onInspectionRetry) {
        void onInspectionRetry();
        return;
      }
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
