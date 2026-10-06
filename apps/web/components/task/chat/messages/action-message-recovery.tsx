"use client";

import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { SessionRecoveryNotice } from "@/components/task/ensure-session-error";
import { useSessionRecoveryActions } from "@/hooks/domains/session/use-session-recovery-actions";
import type { SessionRecoveryAction } from "@/lib/services/session-recovery-service";
import type { MessageAction } from "@/components/task/chat/types";
import { RecoveryActions, type RecoveryChoice } from "@/components/task/recovery-actions";
import { sanitizeSessionErrorDetails } from "@/lib/session-error-details";
import { SessionErrorDetails } from "@/components/task/session-error-details";
import { ManagedCloneRelocationConfirmation } from "@/components/task/chat/managed-clone-relocation-confirmation";
import { ActionButton } from "./action-message-actions";

export function sessionRecoveryAction(action: MessageAction): SessionRecoveryAction | null {
  if (action.type !== "ws_request" || !action.params) return null;
  if (action.params.method !== "session.recover") return null;
  const payload = action.params.payload;
  if (!payload || typeof payload !== "object") return null;
  const recoveryAction = (payload as { action?: unknown }).action;
  switch (recoveryAction) {
    case "resume":
    case "resume_new_branch":
    case "fresh_start":
    case "runtime_retry":
    case "relocate_and_resume":
      return recoveryAction;
    default:
      return null;
  }
}

function recoveryActionLabel(
  action: SessionRecoveryAction,
  t: ReturnType<typeof useTranslation>["t"],
) {
  if (action === "resume") return t("task:resumeSession");
  if (action === "fresh_start") return t("task:startFreshSession");
  if (action === "resume_new_branch") return t("task:continueOnNewBranch");
  if (action === "relocate_and_resume") return t("task:managedCloneRelocateResume");
  return t("chat:managedRuntimeRetry");
}

function recoveryActionTooltip(action: MessageAction, t: ReturnType<typeof useTranslation>["t"]) {
  switch (action.tooltip_key) {
    case "sessionRecoveryResumeDescription":
      return t("task:sessionRecoveryResumeDescription");
    case "sessionRecoveryFreshDescription":
      return t("task:sessionRecoveryFreshDescription");
    case "sessionRecoveryCorruptedDescription":
      return t("task:sessionRecoveryCorruptedDescription");
    default:
      return action.tooltip;
  }
}

function buildRecoveryChoices({
  actions,
  t,
  managedCloneRecoveryStamp,
  providerRestoredResumeEligible,
  recoveryError,
  branchDetails,
  onRecoveryAction,
  onRestore,
  onNewBranch,
  onRelocationConfirm,
}: {
  actions: MessageAction[];
  t: ReturnType<typeof useTranslation>["t"];
  managedCloneRecoveryStamp: string | null;
  providerRestoredResumeEligible: boolean;
  recoveryError: Error | null;
  branchDetails: unknown;
  onRecoveryAction: (action: SessionRecoveryAction) => void;
  onRestore: () => void;
  onNewBranch: () => void;
  onRelocationConfirm: () => void;
}): RecoveryChoice[] {
  const choices: RecoveryChoice[] = managedCloneRecoveryStamp
    ? [
        {
          kind: "relocate_and_resume",
          label: t("task:managedCloneRelocateResume"),
          testId: "managed-clone-relocate-button",
          onClick: onRelocationConfirm,
        },
      ]
    : actions.flatMap((action) => {
        const kind = sessionRecoveryAction(action);
        return kind
          ? [
              {
                kind,
                label: recoveryActionLabel(kind, t),
                disclosure:
                  kind === "resume" && providerRestoredResumeEligible
                    ? t("task:providerRestoredResumeDisclosure")
                    : undefined,
                testId: action.test_id,
                tooltip: recoveryActionTooltip(action, t),
                onClick: () => onRecoveryAction(kind),
              },
            ]
          : [];
      });
  if (recoveryError && !managedCloneRecoveryStamp)
    choices.push({
      kind: "restore",
      label: t("task:restoreReadOnlyWorkspace"),
      testId: "recovery-restore-workspace-button",
      onClick: onRestore,
    });
  if (
    !managedCloneRecoveryStamp &&
    branchDetails &&
    !choices.some((choice) => choice.kind === "resume_new_branch")
  )
    choices.push({
      kind: "resume_new_branch",
      label: t("task:continueOnNewBranch"),
      testId: "recovery-new-branch-button",
      onClick: onNewBranch,
    });
  return choices;
}

export function SessionRecoveryActionButtons({
  actions,
  taskId,
  sessionId,
  errorStamp,
  onRecoveryRequested,
}: {
  actions: MessageAction[];
  taskId: string;
  sessionId: string;
  errorStamp?: string;
  onRecoveryRequested: () => void;
}) {
  const { t } = useTranslation();
  const {
    busyAction,
    recoveryError,
    branchDetails,
    guardDetails,
    recoveryNotice,
    managedCloneRecoveryStamp,
    providerRestoredResumeEligible,
    handleRecover,
    handleRestore,
    handleNewBranch,
    handleManagedCloneRelocation,
  } = useSessionRecoveryActions({ taskId, sessionId, errorStamp });
  const [relocationConfirmationOpen, setRelocationConfirmationOpen] = useState(false);

  const onRecoveryAction = useCallback(
    async (action: SessionRecoveryAction) => {
      if (await handleRecover(action)) onRecoveryRequested();
    },
    [handleRecover, onRecoveryRequested],
  );
  const choices = buildRecoveryChoices({
    actions,
    t,
    managedCloneRecoveryStamp,
    providerRestoredResumeEligible,
    recoveryError,
    branchDetails,
    onRecoveryAction: (action) => void onRecoveryAction(action),
    onRestore: () => void handleRestore(),
    onNewBranch: () =>
      void handleNewBranch().then((success) => {
        if (success) onRecoveryRequested();
      }),
    onRelocationConfirm: () => setRelocationConfirmationOpen(true),
  });
  return (
    <>
      {recoveryError && (
        <div data-testid="session-recovery-error" className="mt-2 min-w-0 text-xs">
          <p role="status">
            {guardDetails
              ? sanitizeSessionErrorDetails(recoveryError.message, 240)
              : t("task:failedToResumeSession")}
          </p>
          <SessionErrorDetails>{recoveryError.message}</SessionErrorDetails>
        </div>
      )}
      {recoveryNotice && <SessionRecoveryNotice message={recoveryNotice} />}
      <RecoveryActions
        actions={choices}
        preferred={
          managedCloneRecoveryStamp
            ? "relocate_and_resume"
            : actions.map(sessionRecoveryAction).find((kind) => kind !== null)
        }
        busy={busyAction !== null}
        busyAction={busyAction}
        blocked={Boolean(guardDetails && !guardDetails.retryable)}
      />
      {actions
        .filter((action) => !sessionRecoveryAction(action))
        .map((action, index) => (
          <ActionButton key={action.test_id ?? index} action={action} messageTaskId={taskId} />
        ))}
      <ManagedCloneRelocationConfirmation
        open={relocationConfirmationOpen}
        targetKey={`${sessionId}:${managedCloneRecoveryStamp ?? ""}`}
        onOpenChange={setRelocationConfirmationOpen}
        onConfirm={() =>
          void handleManagedCloneRelocation().then((success) => {
            if (success) onRecoveryRequested();
          })
        }
        disabled={busyAction !== null}
      />
    </>
  );
}
