"use client";

import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { SessionRecoveryNotice } from "@/components/task/ensure-session-error";
import { useSessionRecoveryActions } from "@/hooks/domains/session/use-session-recovery-actions";
import type {
  ContextContinuationDetails,
  SessionRecoveryAction,
} from "@/lib/services/session-recovery-service";
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
    case "continue_from_history":
    case "retry_connection":
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
  if (action === "continue_from_history") return t("task:continueFromHistory");
  if (action === "retry_connection") return t("task:retryConnection");
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

function addHistoryContinuationChoice(
  choices: RecoveryChoice[],
  enabled: boolean,
  onContinueFromHistory: () => void,
  label: string,
) {
  if (!enabled || choices.some((choice) => choice.kind === "continue_from_history")) return;
  choices.push({
    kind: "continue_from_history",
    label,
    testId: "recovery-continue-from-history-button",
    onClick: onContinueFromHistory,
  });
}

function choicesFromMessageActions(
  actions: MessageAction[],
  t: ReturnType<typeof useTranslation>["t"],
  providerRestoredResumeEligible: boolean,
  onRecoveryAction: (action: SessionRecoveryAction) => void | Promise<void>,
): RecoveryChoice[] {
  return actions.flatMap((action) => {
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
            onClick: () => void onRecoveryAction(kind),
          },
        ]
      : [];
  });
}

function buildRecoveryChoices({
  actions,
  t,
  managedCloneRecoveryStamp,
  providerRestoredResumeEligible,
  recoveryError,
  branchDetails,
  continuationDetails,
  onRecoveryAction,
  onRestore,
  onNewBranch,
  onContinueFromHistory,
  onRelocationConfirm,
}: {
  actions: MessageAction[];
  t: ReturnType<typeof useTranslation>["t"];
  managedCloneRecoveryStamp: string | null;
  providerRestoredResumeEligible: boolean;
  recoveryError: Error | null;
  branchDetails: unknown;
  continuationDetails: ContextContinuationDetails | null;
  onRecoveryAction: (action: SessionRecoveryAction) => void;
  onRestore: () => void;
  onNewBranch: () => void;
  onContinueFromHistory: () => void;
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
    : choicesFromMessageActions(actions, t, providerRestoredResumeEligible, onRecoveryAction);
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
  addHistoryContinuationChoice(
    choices,
    !managedCloneRecoveryStamp && Boolean(continuationDetails),
    onContinueFromHistory,
    t("task:continueFromHistory"),
  );
  return choices;
}

function SessionRecoveryFailure({ error, guarded }: { error: Error; guarded: boolean }) {
  const { t } = useTranslation();
  return (
    <div data-testid="session-recovery-error" className="mt-2 min-w-0 text-xs">
      <p role="status">
        {guarded
          ? sanitizeSessionErrorDetails(error.message, 240)
          : t("task:failedToResumeSession")}
      </p>
      <SessionErrorDetails>{error.message}</SessionErrorDetails>
    </div>
  );
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
    continuationDetails,
    recoveryNotice,
    managedCloneRecoveryStamp,
    workspaceRecovery,
    workspaceRecoveryRepositoryName,
    workspaceRecoveryStatusCheck,
    checkWorkspaceRecoveryStatus,
    providerRestoredResumeEligible,
    handleRecover,
    handleRestore,
    handleNewBranch,
    handleContinueFromHistory,
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
    continuationDetails,
    onRecoveryAction: (action) => void onRecoveryAction(action),
    onRestore: () => void handleRestore(),
    onNewBranch: () =>
      void handleNewBranch().then((success) => {
        if (success) onRecoveryRequested();
      }),
    onContinueFromHistory: () =>
      void handleContinueFromHistory().then((success) => {
        if (success) onRecoveryRequested();
      }),
    onRelocationConfirm: () => setRelocationConfirmationOpen(true),
  });
  return (
    <>
      {recoveryError && (
        <SessionRecoveryFailure error={recoveryError} guarded={Boolean(guardDetails)} />
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
        workspaceRecovery={workspaceRecovery}
        workspaceRecoveryRepositoryName={workspaceRecoveryRepositoryName}
        workspaceRecoveryStatusCheck={workspaceRecoveryStatusCheck}
        onCheckWorkspaceRecoveryStatus={() => void checkWorkspaceRecoveryStatus()}
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
