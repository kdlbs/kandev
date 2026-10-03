"use client";

import { useState, memo, type ReactElement } from "react";
import { useSessionComposerRecovery } from "../session-recovery-context";
import { useAppStore } from "@/components/state-provider";
import { Trans, useTranslation } from "react-i18next";
import { IconAlertTriangle } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import {
  useActionMessageSession,
  useAgentBootOutcomeAfterMessage,
  useRunningNoticeResolved,
} from "./action-message-state";
import type { Message, TaskSessionState } from "@/lib/types/http";
import type { MessageAction } from "@/components/task/chat/types";
import { ActionMessageDetails, type ActionMeta } from "./action-message-details";
import { ManagedRuntimeStartupRecoveryMessage } from "./managed-runtime-startup-recovery-message";
import { ManagedRuntimeNpmRecoveryMessage } from "./managed-runtime-npm-recovery-message";
import { formatDateTime } from "@/lib/i18n/formats";
import { TransientRetryNotice } from "./transient-retry-notice";
import { ActionButtons } from "./action-message-actions";
import { SessionRecoveryActionButtons, sessionRecoveryAction } from "./action-message-recovery";
import { RecoveryHistory, resolveRecoveryHistoryState } from "./action-message-recovery-history";
import { readableFailureSummary } from "./action-message-utils";
import {
  lastAgentErrorStamp,
  readLastAgentError,
  readLastAgentErrorIncludingDismissed,
  type LastAgentError,
} from "@/lib/session-last-agent-error";
import { legacyRecoveryMessageMatchesError } from "@/lib/session-recovery-presentation";
import { interruptionRecoveryKey, retryNoticeVisible } from "./interruption-recovery-feedback";

function isSessionActive(state?: TaskSessionState) {
  return state === "RUNNING" || state === "STARTING" || state === "COMPLETED";
}

function currentSessionRecoveryError(sessionMetadata: Record<string, unknown> | null) {
  if (!sessionMetadata) return null;
  return readLastAgentError(sessionMetadata);
}

function isCurrentRecoveryMessage(
  isRecoveryMessage: boolean,
  messageRecoveryStamp: string | undefined,
  currentRecoveryError: LastAgentError | null,
  comment: Message,
  sessionMetadata: Record<string, unknown> | null | undefined,
) {
  if (!isRecoveryMessage) return true;
  if (messageRecoveryStamp) {
    const latestError = readLastAgentErrorIncludingDismissed(sessionMetadata);
    if (!latestError || latestError.dismissedAt) return false;
    return lastAgentErrorStamp(latestError) === messageRecoveryStamp;
  }
  if (!currentRecoveryError) return true;
  return legacyRecoveryMessageMatchesError(
    comment.content,
    comment.created_at,
    currentRecoveryError,
  );
}

function shouldShowRecoveryActions({
  isRecoveryMessage,
  isCurrentRecovery,
  recoveryResolvedDurably,
  agentRebooted,
  recoveryRequested,
  recoveryFailedAgain,
  sessionState,
}: {
  isRecoveryMessage: boolean;
  isCurrentRecovery: boolean;
  recoveryResolvedDurably: boolean;
  agentRebooted: boolean;
  recoveryRequested: boolean;
  recoveryFailedAgain: boolean;
  sessionState?: TaskSessionState;
}) {
  if (!isRecoveryMessage) return true;
  if (!isCurrentRecovery || recoveryResolvedDurably || agentRebooted) return false;
  if (recoveryRequested && !recoveryFailedAgain) return false;
  return !isSessionActive(sessionState);
}

export const ActionMessage = memo(function ActionMessage({ comment }: { comment: Message }) {
  const owner = useSessionComposerRecovery(comment.session_id);
  const metadata = comment.metadata as ActionMeta | undefined;
  const runningNoticeResolved = useRunningNoticeResolved(
    comment,
    metadata?.action_visibility === "running" && comment.type === "status",
  );
  const sessionMetadata = useAppStore((state) =>
    comment.session_id ? state.taskSessions.items[comment.session_id]?.metadata : undefined,
  );
  if (runningNoticeResolved) return null;
  const historyState = resolveRecoveryHistoryState(
    comment,
    metadata,
    owner?.model?.messageId,
    sessionMetadata,
  );
  if (historyState) {
    return (
      <RecoveryHistory
        comment={comment}
        metadata={metadata}
        activeOwner={historyState.activeOwner}
      />
    );
  }
  return <ActionMessageControls comment={comment} />;
});

const ActionMessageControls = memo(function ActionMessageControls({
  comment,
}: {
  comment: Message;
}) {
  // Read session state from the store instead of receiving it as a prop, so a
  // state transition doesn't re-render every message in the list (only the
  // rare action messages that actually depend on it).
  const { t } = useTranslation();
  const { sessionState, sessionError, sessionMetadata, activeTurnId } = useActionMessageSession(
    comment.session_id,
  );
  const metadata = comment.metadata as ActionMeta | undefined;
  const message = comment.content || t("task:anErrorOccurred");
  const isRecoveryMessage = metadata?.recovery_actions === true;
  const messageRecoveryStamp = metadata?.recovery_stamp ?? metadata?.error_stamp;
  // The recovery acknowledgment lives here, on the message row that stays
  // mounted, not on SettledFailureMessage: a successful resume drives the
  // session through STARTING/RUNNING (which unmounts the card via
  // isSessionActive) and back to WAITING_FOR_INPUT once the agent is idle.
  // Local state on the card would reset on that remount and the recovery banner
  // would reappear until the next user message.
  const [recoveryRequested, setRecoveryRequested] = useState(false);
  // Durable counterpart to the click acknowledgment: the transcript itself
  // records that the agent booted again after this failure. It survives a
  // reload or task switch and also covers auto-resume-on-open, where the card
  // would otherwise linger until the next prompt flipped the session to RUNNING.
  const {
    agentRebooted,
    agentBootFailed,
    recoveryResolved: recoveryResolvedDurably,
  } = useAgentBootOutcomeAfterMessage(comment, isRecoveryMessage, sessionMetadata);
  // The click acknowledgment only covers the wait for that outcome. A recovery
  // that came back failed — a failed boot row, or a session driven to FAILED —
  // must surface its card again, buttons included, or the retry is unreachable.
  const recoveryFailedAgain = agentBootFailed || sessionState === "FAILED";
  const currentRecoveryError = currentSessionRecoveryError(sessionMetadata ?? null);
  const isCurrentRecovery = isCurrentRecoveryMessage(
    isRecoveryMessage,
    messageRecoveryStamp,
    currentRecoveryError,
    comment,
    sessionMetadata,
  );
  const recoveryActionsVisible = shouldShowRecoveryActions({
    isRecoveryMessage,
    isCurrentRecovery,
    recoveryResolvedDurably,
    agentRebooted,
    recoveryRequested,
    recoveryFailedAgain,
    sessionState,
  });

  if (metadata?.action_visibility === "running") {
    if (sessionState === "RUNNING" && comment.turn_id && activeTurnId === comment.turn_id) {
      return (
        <RunningActionNotice
          actions={metadata.actions}
          message={message}
          taskId={comment.task_id}
        />
      );
    }
    // A terminal error may have been persisted with the old running metadata
    // shape. Let it use the settled renderer instead of hiding the diagnostic.
    if (comment.type !== "error") {
      return null;
    }
  }

  return (
    <SettledActionMessage
      metadata={metadata}
      message={message}
      sessionError={sessionError}
      sessionState={sessionState}
      taskId={comment.task_id}
      sessionId={comment.session_id}
      recoveryActionsVisible={recoveryActionsVisible}
      onRecoveryRequested={() => setRecoveryRequested(true)}
    />
  );
});

function SettledActionMessage({
  metadata,
  message,
  sessionError,
  sessionState,
  taskId,
  sessionId,
  recoveryActionsVisible,
  onRecoveryRequested,
}: {
  metadata: ActionMeta | undefined;
  message: string;
  sessionError?: string;
  sessionState?: TaskSessionState;
  taskId?: string;
  sessionId?: string;
  recoveryActionsVisible: boolean;
  onRecoveryRequested: () => void;
}) {
  if (metadata?.retrying) {
    return retryNoticeVisible(sessionState, metadata) ? (
      <TransientRetryNotice metadata={metadata} taskId={taskId} />
    ) : null;
  }
  if (isSessionActive(sessionState) && metadata?.recovery_actions !== true) return null;

  return (
    <SettledFailureMessage
      metadata={metadata}
      message={message}
      sessionError={sessionError}
      taskId={taskId}
      sessionId={sessionId}
      recoveryActionsVisible={recoveryActionsVisible}
      onRecoveryRequested={onRecoveryRequested}
    />
  );
}

function SettledFailureMessage({
  metadata,
  message,
  sessionError,
  taskId,
  sessionId,
  recoveryActionsVisible,
  onRecoveryRequested,
}: {
  metadata: ActionMeta | undefined;
  message: string;
  sessionError?: string;
  taskId?: string;
  sessionId?: string;
  recoveryActionsVisible: boolean;
  onRecoveryRequested: () => void;
}) {
  const { t } = useTranslation();
  const safeMessage =
    metadata?.failure_kind === "provider_interrupted"
      ? t(interruptionRecoveryKey(metadata), { count: metadata.attempts_started })
      : readableFailureSummary(message);
  const needsDetails = safeMessage === null;
  const renderedMetadata = recoveryActionsVisible ? metadata : withoutRecoveryActions(metadata);

  const specialRecovery = renderSpecialRecovery({
    metadata: renderedMetadata,
    message,
    sessionError,
    taskId,
    onRecoveryRequested,
  });
  if (specialRecovery) return specialRecovery;

  const iconClass = metadata?.variant === "warning" ? "text-amber-500" : "text-red-500";
  const textClass =
    metadata?.variant === "warning"
      ? "text-amber-600 dark:text-amber-400"
      : "text-red-600 dark:text-red-400";
  return (
    <div className="w-full" data-testid="session-recovery-action-message">
      <div className="flex items-start gap-3 w-full rounded px-2 py-1 -mx-2">
        <div className="flex-shrink-0 mt-0.5">
          <IconAlertTriangle className={cn("h-4 w-4", iconClass)} />
        </div>
        <div className="flex-1 min-w-0 pt-0.5">
          <div className={cn("text-xs wrap-anywhere", textClass)}>
            {needsDetails ? t("task:anErrorOccurred") : safeMessage}
          </div>
          {renderSettledActionButtons({
            actions: renderedMetadata?.actions,
            taskId,
            sessionId,
            isRecoveryMessage: metadata?.recovery_actions === true,
            errorStamp: metadata?.error_stamp ?? metadata?.recovery_stamp,
            onRecoveryRequested,
          })}
          <ActionMessageDetails
            metadata={failureDetailsMetadata(renderedMetadata, needsDetails, message)}
          />
        </div>
      </div>
    </div>
  );
}

function failureDetailsMetadata(
  metadata: ActionMeta | undefined,
  needsDetails: boolean,
  message: string,
) {
  return needsDetails ? { ...metadata, error_output: metadata?.error_output || message } : metadata;
}

function withoutRecoveryActions(metadata: ActionMeta | undefined): ActionMeta | undefined {
  if (!metadata) return undefined;
  return { ...metadata, actions: undefined };
}

function renderSettledActionButtons({
  actions,
  taskId,
  sessionId,
  isRecoveryMessage,
  errorStamp,
  onRecoveryRequested,
}: {
  actions?: MessageAction[];
  taskId?: string;
  sessionId?: string;
  isRecoveryMessage: boolean;
  errorStamp?: string;
  onRecoveryRequested: () => void;
}): ReactElement | null {
  if (!actions || actions.length === 0) return null;
  const hasSessionRecoveryAction = actions.some((action) => sessionRecoveryAction(action));
  if (hasSessionRecoveryAction && taskId && sessionId) {
    return (
      <SessionRecoveryActionButtons
        actions={actions}
        taskId={taskId}
        sessionId={sessionId}
        errorStamp={errorStamp}
        onRecoveryRequested={onRecoveryRequested}
      />
    );
  }
  return (
    <ActionButtons
      actions={actions}
      taskId={taskId}
      onRecoveryRequested={isRecoveryMessage ? onRecoveryRequested : undefined}
    />
  );
}

function renderSpecialRecovery({
  metadata,
  message,
  sessionError,
  taskId,
  onRecoveryRequested,
}: {
  metadata: ActionMeta | undefined;
  message: string;
  sessionError?: string;
  taskId?: string;
  onRecoveryRequested: () => void;
}): ReactElement | null {
  if (metadata?.failure_kind === "missing_pr_branch") {
    return (
      <MissingBranchRecovery
        metadata={metadata}
        taskId={taskId}
        fallbackMessage={message}
        technicalDetails={sessionError}
      />
    );
  }
  if (metadata?.failure_kind === "provider_quota_limited") {
    return (
      <ProviderQuotaRecovery
        metadata={metadata}
        taskId={taskId}
        onRecoveryRequested={onRecoveryRequested}
      />
    );
  }
  if (
    metadata?.failure_kind === "managed_runtime_npm_resolution" ||
    metadata?.failure_kind === "managed_runtime_npm_policy"
  ) {
    return (
      <ManagedRuntimeNpmRecoveryMessage
        metadata={metadata}
        taskId={taskId}
        onRecoveryRequested={onRecoveryRequested}
      />
    );
  }
  if (metadata?.failure_kind === "managed_runtime_startup") {
    return (
      <ManagedRuntimeStartupRecoveryMessage
        metadata={metadata}
        taskId={taskId}
        onRecoveryRequested={onRecoveryRequested}
      />
    );
  }
  return null;
}

function ProviderQuotaRecovery({
  metadata,
  taskId,
  onRecoveryRequested,
}: {
  metadata: ActionMeta;
  taskId?: string;
  onRecoveryRequested: () => void;
}) {
  const { t } = useTranslation();
  const provider = metadata.provider_name?.trim() || t("chat:providerQuotaProviderFallback");
  const model = metadata.model_id?.trim();
  const resetDate = metadata.reset_at ? new Date(metadata.reset_at) : undefined;
  const resetAt = resetDate && !Number.isNaN(resetDate.getTime()) ? formatDateTime(resetDate) : "";

  return (
    <section
      data-testid="provider-quota-recovery"
      role="alert"
      className="w-full min-w-0 rounded-md border border-amber-500/25 bg-amber-500/[0.06] p-3 sm:p-4"
    >
      <div className="flex min-w-0 items-start gap-3">
        <IconAlertTriangle
          className="mt-0.5 h-4 w-4 flex-shrink-0 text-amber-500"
          aria-hidden="true"
        />
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-medium text-foreground">
            {t("chat:providerQuotaTitle", { provider })}
          </h3>
          <div className="mt-1 space-y-1 text-xs leading-relaxed text-muted-foreground">
            {model && <p>{t("chat:providerQuotaModel", { model })}</p>}
            <p>
              {resetAt
                ? t("chat:providerQuotaReset", { resetAt })
                : t("chat:providerQuotaResetUnknown")}
            </p>
          </div>
          <ActionMessageDetails metadata={metadata} />
          {metadata.actions && metadata.actions.length > 0 && (
            <ActionButtons
              actions={metadata.actions}
              taskId={taskId}
              onRecoveryRequested={onRecoveryRequested}
            />
          )}
        </div>
      </div>
    </section>
  );
}

function RunningActionNotice({
  actions,
  message,
  taskId,
}: {
  actions?: MessageAction[];
  message: string;
  taskId?: string;
}) {
  return (
    <div
      data-testid="running-action-notice"
      className="flex min-w-0 items-center gap-2 py-1 text-muted-foreground"
    >
      <span className="min-w-0 flex-1 truncate text-xs">{message}</span>
      {actions && actions.length > 0 && <ActionButtons actions={actions} taskId={taskId} compact />}
    </div>
  );
}

function MissingBranchRecovery({
  metadata,
  taskId,
  fallbackMessage,
  technicalDetails,
}: {
  metadata: ActionMeta;
  taskId?: string;
  fallbackMessage: string;
  technicalDetails?: string;
}) {
  const { t } = useTranslation();
  const branch = metadata.missing_branch?.trim();
  return (
    <section
      data-testid="missing-branch-recovery"
      role="alert"
      className="w-full min-w-0 rounded-md border border-amber-500/25 bg-amber-500/[0.06] p-3 sm:p-4"
    >
      <div className="flex min-w-0 items-start gap-3">
        <IconAlertTriangle
          className="mt-0.5 h-4 w-4 flex-shrink-0 text-amber-500"
          aria-hidden="true"
        />
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-medium text-foreground">
            {t("task:branchIsNoLongerAvailable")}
          </h3>
          <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
            {branch ? (
              <Trans i18nKey="task:thisTaskPointsToBranch" values={{ branch }}>
                This task points to <code className="break-all text-foreground">{branch}</code>, but
                that branch could not be found on the remote repository. It may have been merged or
                deleted.
              </Trans>
            ) : (
              fallbackMessage
            )}
          </p>
          <ActionMessageDetails metadata={metadata} technicalDetails={technicalDetails} />
          {metadata.actions && metadata.actions.length > 0 && (
            <ActionButtons actions={metadata.actions} taskId={taskId} />
          )}
        </div>
      </div>
    </section>
  );
}
