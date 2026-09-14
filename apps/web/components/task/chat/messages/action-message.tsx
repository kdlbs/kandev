"use client";

import { useState, useEffect, useMemo, memo, type ReactElement } from "react";
import { Trans, useTranslation } from "react-i18next";
import { IconAlertTriangle } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { useActionMessageSession, useAgentBootOutcomeAfterMessage } from "./action-message-state";
import type { Message, TaskSessionState } from "@/lib/types/http";
import type { MessageAction } from "@/components/task/chat/types";
import { ActionMessageDetails, type ActionMeta } from "./action-message-details";
import { formatDateTime } from "@/lib/i18n/formats";
import { parseRetryAt, retryCountdownLabel } from "./transient-retry";
import { hasSessionRecoveryResolutionAfter } from "@/hooks/processed-message-filtering";
import { ActionButtons } from "./action-message-actions";
import { SessionRecoveryActionButtons, sessionRecoveryAction } from "./action-message-recovery";

function isSessionActive(state?: TaskSessionState) {
  return state === "RUNNING" || state === "STARTING" || state === "COMPLETED";
}

export const ActionMessage = memo(function ActionMessage({ comment }: { comment: Message }) {
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
  const { agentRebooted, agentBootFailed } = useAgentBootOutcomeAfterMessage(
    comment,
    isRecoveryMessage,
  );
  const recoveryResolvedDurably = isRecoveryMessage
    ? hasSessionRecoveryResolutionAfter(sessionMetadata, comment.created_at)
    : false;
  // The click acknowledgment only covers the wait for that outcome. A recovery
  // that came back failed — a failed boot row, or a session driven to FAILED —
  // must surface its card again, buttons included, or the retry is unreachable.
  const recoveryFailedAgain = agentBootFailed || sessionState === "FAILED";

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
      recoveryResolved={
        recoveryResolvedDurably || agentRebooted || (recoveryRequested && !recoveryFailedAgain)
      }
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
  recoveryResolved,
  onRecoveryRequested,
}: {
  metadata: ActionMeta | undefined;
  message: string;
  sessionError?: string;
  sessionState?: TaskSessionState;
  taskId?: string;
  sessionId?: string;
  recoveryResolved: boolean;
  onRecoveryRequested: () => void;
}) {
  // A retry card is persisted against the failed turn, so hide it while the
  // replacement turn is starting or running to avoid showing stale progress.
  if (isSessionActive(sessionState)) return null;

  if (metadata?.retrying) {
    return <TransientRetryNotice metadata={metadata} taskId={taskId} />;
  }

  return (
    <SettledFailureMessage
      metadata={metadata}
      message={message}
      sessionError={sessionError}
      sessionState={sessionState}
      taskId={taskId}
      sessionId={sessionId}
      recoveryResolved={recoveryResolved}
      onRecoveryRequested={onRecoveryRequested}
    />
  );
}

function SettledFailureMessage({
  metadata,
  message,
  sessionError,
  sessionState,
  taskId,
  sessionId,
  recoveryResolved,
  onRecoveryRequested,
}: {
  metadata: ActionMeta | undefined;
  message: string;
  sessionError?: string;
  sessionState?: TaskSessionState;
  taskId?: string;
  sessionId?: string;
  recoveryResolved: boolean;
  onRecoveryRequested: () => void;
}) {
  // A waiting session can still need recovery, so only hide this persisted card
  // once its own recovery resolved: either the Resume click was acknowledged or
  // the transcript shows the agent booted again after this failure. A resumed
  // agent settles at WAITING_FOR_INPUT, which isSessionActive deliberately
  // excludes, so without that second signal the card outlives the failure it
  // describes.
  if (isSessionActive(sessionState) || (metadata?.recovery_actions && recoveryResolved))
    return null;

  const specialRecovery = renderSpecialRecovery({
    metadata,
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
    <div className="w-full">
      <div className="flex items-start gap-3 w-full rounded px-2 py-1 -mx-2">
        <div className="flex-shrink-0 mt-0.5">
          <IconAlertTriangle className={cn("h-4 w-4", iconClass)} />
        </div>
        <div className="flex-1 min-w-0 pt-0.5">
          <div className={cn("text-xs break-words", textClass)}>{message}</div>
          <ActionMessageDetails metadata={metadata} />
          {renderSettledActionButtons({
            actions: metadata?.actions,
            taskId,
            sessionId,
            isRecoveryMessage: metadata?.recovery_actions === true,
            onRecoveryRequested,
          })}
        </div>
      </div>
    </div>
  );
}

function renderSettledActionButtons({
  actions,
  taskId,
  sessionId,
  isRecoveryMessage,
  onRecoveryRequested,
}: {
  actions?: MessageAction[];
  taskId?: string;
  sessionId?: string;
  isRecoveryMessage: boolean;
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

function transientRetryReasonKey(failureCode?: string) {
  switch (failureCode) {
    case "model_capacity":
      return "chat:transientRetryReasonModelCapacity";
    case "network_unavailable":
      return "chat:transientRetryReasonNetworkUnavailable";
    case "provider_overloaded":
      return "chat:transientRetryReasonProviderOverloaded";
    case "provider_unavailable":
      return "chat:transientRetryReasonProviderUnavailable";
    case "rate_limited":
      return "chat:transientRetryReasonRateLimited";
    case "agent_transport_lost":
      return "chat:transientRetryReasonAgentTransportLost";
    default:
      return "chat:transientRetryReasonGeneric";
  }
}

function transientRetryContext(
  t: ReturnType<typeof useTranslation>["t"],
  provider?: string,
  model?: string,
) {
  if (!provider && !model) return null;
  if (provider && model) return t("chat:transientRetryProviderModel", { provider, model });
  if (provider) return t("chat:transientRetryProvider", { provider });
  return t("chat:transientRetryModel", { model });
}

function TransientRetryNotice({ metadata, taskId }: { metadata: ActionMeta; taskId?: string }) {
  const { t } = useTranslation();
  const retryAt = parseRetryAt(metadata.retry_at);
  const fallbackDeadline = useMemo(
    () => Date.now() + Math.max(0, metadata.retry_in_seconds ?? 0) * 1_000,
    [metadata.attempt, metadata.retry_in_seconds],
  );
  const deadline = retryAt ?? fallbackDeadline;
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    setNow(Date.now());
    const timer = window.setInterval(() => setNow(Date.now()), 1_000);
    return () => window.clearInterval(timer);
  }, [retryAt, metadata.retry_in_seconds]);

  const remaining = Math.max(0, deadline - now);
  const reasonKey = transientRetryReasonKey(metadata.failure_code);
  const provider = metadata.provider_name?.trim();
  const model = metadata.model_id?.trim();
  const context = transientRetryContext(t, provider, model);
  const countdown =
    remaining > 0
      ? t("chat:transientRetryIn", { countdown: retryCountdownLabel(remaining) })
      : t("chat:transientRetryNow");

  return (
    <section
      data-testid="transient-retry-card"
      role="status"
      aria-live="polite"
      className="flex w-full min-w-0 items-center gap-2 rounded-md border border-amber-500/25 bg-amber-500/[0.06] px-2 py-1.5 sm:gap-3 sm:px-3"
    >
      <IconAlertTriangle className="h-4 w-4 flex-shrink-0 text-amber-500" aria-hidden="true" />
      <div className="min-w-0 flex-1 text-xs">
        <div className="flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5 text-amber-600 dark:text-amber-400">
          <span>{t(reasonKey)}</span>
          <span aria-label={t("chat:transientRetryCountdownLabel")}>{countdown}</span>
        </div>
        {context && <div className="mt-0.5 truncate text-muted-foreground">{context}</div>}
        <div className="mt-0.5 text-muted-foreground">
          {t("chat:transientRetryAttempt", {
            attempt: metadata.attempt ?? 1,
            maxAttempts: metadata.max_attempts ?? 1,
          })}
        </div>
      </div>
      {metadata.actions && metadata.actions.length > 0 && (
        <ActionButtons actions={metadata.actions} taskId={taskId} compact />
      )}
    </section>
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
  if (metadata?.failure_kind === "managed_runtime_npm_resolution") {
    return (
      <ManagedRuntimeNpmRecovery
        metadata={metadata}
        taskId={taskId}
        onRecoveryRequested={onRecoveryRequested}
      />
    );
  }
  return null;
}

function ManagedRuntimeNpmRecovery({
  metadata,
  taskId,
  onRecoveryRequested,
}: {
  metadata: ActionMeta;
  taskId?: string;
  onRecoveryRequested: () => void;
}) {
  const { t } = useTranslation();
  const actions = metadata.actions?.slice(0, 1) ?? [];
  return (
    <section
      data-testid="managed-runtime-npm-recovery"
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
            {t("chat:managedRuntimeNpmTitle")}
          </h3>
          <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
            {t("chat:managedRuntimeNpmBody")}
          </p>
          <ActionMessageDetails metadata={metadata} />
          {actions.length > 0 && (
            <ActionButtons
              actions={actions}
              taskId={taskId}
              onRecoveryRequested={onRecoveryRequested}
              labelOverride={t("chat:managedRuntimeRetry")}
            />
          )}
        </div>
      </div>
    </section>
  );
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
