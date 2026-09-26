"use client";

import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { SessionRecoveryNotice } from "@/components/task/ensure-session-error";
import { useSessionRecoveryActions } from "@/hooks/domains/session/use-session-recovery-actions";
import type { SessionRecoveryAction } from "@/lib/services/session-recovery-service";
import type { MessageAction } from "@/components/task/chat/types";
import { RecoveryActions, type RecoveryChoice } from "@/components/task/recovery-actions";
import { sanitizeSessionErrorDetails } from "@/lib/session-error-details";
import { SessionErrorDetails } from "@/components/task/session-error-details";
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
  return t("chat:managedRuntimeRetry");
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
    handleRecover,
    handleRestore,
    handleNewBranch,
  } = useSessionRecoveryActions({ taskId, sessionId, errorStamp });

  const onRecoveryAction = useCallback(
    async (action: SessionRecoveryAction) => {
      if (await handleRecover(action)) onRecoveryRequested();
    },
    [handleRecover, onRecoveryRequested],
  );
  const choices: RecoveryChoice[] = actions.flatMap((action) => {
    const kind = sessionRecoveryAction(action);
    return kind
      ? [
          {
            kind,
            label: recoveryActionLabel(kind, t),
            testId: action.test_id,
            tooltip: action.tooltip,
            onClick: () => void onRecoveryAction(kind),
          },
        ]
      : [];
  });
  if (recoveryError)
    choices.push({
      kind: "restore",
      label: t("task:restoreReadOnlyWorkspace"),
      testId: "recovery-restore-workspace-button",
      onClick: () => void handleRestore(),
    });
  if (branchDetails && !choices.some((choice) => choice.kind === "resume_new_branch"))
    choices.push({
      kind: "resume_new_branch",
      label: t("task:continueOnNewBranch"),
      testId: "recovery-new-branch-button",
      onClick: () =>
        void handleNewBranch().then((success) => {
          if (success) onRecoveryRequested();
        }),
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
        preferred={actions.map(sessionRecoveryAction).find((kind) => kind !== null)}
        busy={busyAction !== null}
        busyAction={busyAction}
        blocked={Boolean(guardDetails && !guardDetails.retryable)}
      />
      {actions
        .filter((action) => !sessionRecoveryAction(action))
        .map((action, index) => (
          <ActionButton key={action.test_id ?? index} action={action} messageTaskId={taskId} />
        ))}
    </>
  );
}
