"use client";

import { useState, useCallback, useEffect, useMemo, memo, type ReactElement } from "react";
import { Trans, useTranslation } from "react-i18next";
import {
  IconAlertTriangle,
  IconArchive,
  IconTrash,
  IconRefresh,
  IconPlayerPlay,
  IconSparkles,
  IconGitCommit,
  IconX,
  IconChevronDown,
} from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useArchiveAndSwitchTask } from "@/hooks/use-task-actions";
import { useTaskRemoval } from "@/hooks/use-task-removal";
import { deleteTask } from "@/lib/api/domains/kanban-api";
import { AuthMethodsPanel, GenericAuthPanel } from "./auth-methods-panel";
import { RemediationLink } from "@/components/task/remediation-link";
import { HostShellDialog } from "@/components/settings/host-shell-dialog";
import type { Message, TaskSessionState } from "@/lib/types/http";
import type { MessageAction, RecoveryAuthMethod } from "@/components/task/chat/types";
import { formatDateTime } from "@/lib/i18n/formats";
import { parseRetryAt, retryCountdownLabel } from "./transient-retry";

const ICON_MAP: Record<string, React.ElementType> = {
  archive: IconArchive,
  trash: IconTrash,
  refresh: IconRefresh,
  "player-play": IconPlayerPlay,
  sparkles: IconSparkles,
  "git-commit": IconGitCommit,
  "alert-triangle": IconAlertTriangle,
  x: IconX,
};

type ActionMeta = {
  actions?: MessageAction[];
  action_visibility?: "running";
  variant?: string;
  recovery_actions?: boolean;
  is_auth_error?: boolean;
  auth_methods?: RecoveryAuthMethod[];
  error_output?: string;
  failure_kind?: string;
  missing_branch?: string;
  provider_name?: string;
  model_id?: string;
  reset_at?: string;
  remediation_url?: string;
  retrying?: boolean;
  attempt?: number;
  max_attempts?: number;
  retry_in_seconds?: number;
  retry_at?: string;
  failure_code?: string;
};

function isSessionActive(state?: TaskSessionState) {
  return state === "RUNNING" || state === "STARTING" || state === "COMPLETED";
}

export const ActionMessage = memo(function ActionMessage({ comment }: { comment: Message }) {
  // Read session state from the store instead of receiving it as a prop, so a
  // state transition doesn't re-render every message in the list (only the
  // rare action messages that actually depend on it).
  const { t } = useTranslation();
  const { sessionState, sessionError, activeTurnId } = useActionMessageSession(comment.session_id);
  const metadata = comment.metadata as ActionMeta | undefined;
  const message = comment.content || t("task:anErrorOccurred");

  if (metadata?.action_visibility === "running") {
    if (sessionState !== "RUNNING" || !comment.turn_id || activeTurnId !== comment.turn_id) {
      return null;
    }
    return (
      <RunningActionNotice actions={metadata.actions} message={message} taskId={comment.task_id} />
    );
  }

  return (
    <SettledActionMessage
      metadata={metadata}
      message={message}
      sessionError={sessionError}
      sessionState={sessionState}
      taskId={comment.task_id}
    />
  );
});

function SettledActionMessage({
  metadata,
  message,
  sessionError,
  sessionState,
  taskId,
}: {
  metadata: ActionMeta | undefined;
  message: string;
  sessionError?: string;
  sessionState?: TaskSessionState;
  taskId?: string;
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
    />
  );
}

function SettledFailureMessage({
  metadata,
  message,
  sessionError,
  sessionState,
  taskId,
}: {
  metadata: ActionMeta | undefined;
  message: string;
  sessionError?: string;
  sessionState?: TaskSessionState;
  taskId?: string;
}) {
  const [recoveryRequested, setRecoveryRequested] = useState(false);

  // A waiting session can still need recovery, so only hide this persisted
  // card after its own Resume request is acknowledged (or the session starts).
  if (isSessionActive(sessionState) || (metadata?.recovery_actions && recoveryRequested))
    return null;

  const specialRecovery = renderSpecialRecovery({
    metadata,
    message,
    sessionError,
    taskId,
    onRecoveryRequested: () => setRecoveryRequested(true),
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
          {metadata?.actions && metadata.actions.length > 0 && (
            <ActionButtons
              actions={metadata.actions}
              taskId={taskId}
              onRecoveryRequested={
                metadata.recovery_actions ? () => setRecoveryRequested(true) : undefined
              }
            />
          )}
        </div>
      </div>
    </div>
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

function useActionMessageSession(sessionId: Message["session_id"]) {
  const sessionState = useAppStore((state) =>
    sessionId ? (state.taskSessions.items[sessionId]?.state ?? undefined) : undefined,
  );
  const sessionError = useAppStore((state) =>
    sessionId
      ? (state.taskSessions.items[sessionId]?.error_message as string | undefined)
      : undefined,
  );
  const activeTurnId = useAppStore((state) =>
    sessionId ? (state.turns.activeBySession[sessionId] ?? undefined) : undefined,
  );
  return { sessionState, sessionError, activeTurnId };
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

function TechnicalDetails({ children }: { children: string }) {
  const { t } = useTranslation();
  return (
    <details className="mt-2 min-w-0 text-xs text-muted-foreground">
      <summary className="flex min-h-11 cursor-pointer list-none items-center gap-1.5 sm:min-h-8">
        <IconChevronDown className="h-3.5 w-3.5" />
        {t("chat:technicalDetails")}
      </summary>
      <pre className="max-h-[300px] max-w-full overflow-y-auto whitespace-pre-wrap break-words rounded bg-muted/50 p-2 font-mono text-[11px]">
        {children}
      </pre>
    </details>
  );
}

function ActionMessageDetails({
  metadata,
  technicalDetails,
}: {
  metadata: ActionMeta | undefined;
  technicalDetails?: string;
}) {
  const [hostShellOpen, setHostShellOpen] = useState(false);
  const [hostShellCommand, setHostShellCommand] = useState<string | undefined>(undefined);

  // Auth recovery uses the kandev host shell (where the agent CLIs are
  // installed), not the task environment shell - the task env often isn't
  // ready when an auth error fires (no workspace path yet), and the user's
  // agent auth state lives in their home dir on the host anyway.
  const openHostShellWithCommand = useCallback((command: string) => {
    // Trailing newline runs the command immediately. Drop it if you'd rather
    // let the user review first.
    setHostShellCommand(command + "\n");
    setHostShellOpen(true);
  }, []);
  const openHostShell = useCallback(() => {
    setHostShellCommand(undefined);
    setHostShellOpen(true);
  }, []);

  if (!metadata) return null;
  const errorOutput = metadata.error_output || technicalDetails;
  return (
    <>
      {metadata.remediation_url && <RemediationLink url={metadata.remediation_url} />}
      {errorOutput && <TechnicalDetails>{errorOutput}</TechnicalDetails>}
      {metadata.is_auth_error && metadata.auth_methods && metadata.auth_methods.length > 0 && (
        <AuthMethodsPanel
          methods={metadata.auth_methods}
          onOpenTerminal={openHostShellWithCommand}
        />
      )}
      {metadata.is_auth_error && (!metadata.auth_methods || metadata.auth_methods.length === 0) && (
        <GenericAuthPanel onOpenTerminal={openHostShell} />
      )}
      <HostShellDialog
        open={hostShellOpen}
        onOpenChange={setHostShellOpen}
        initialInput={hostShellCommand}
      />
    </>
  );
}

function ActionButtons({
  actions,
  taskId,
  onRecoveryRequested,
  compact = false,
}: {
  actions: MessageAction[];
  taskId?: string;
  onRecoveryRequested?: () => void;
  compact?: boolean;
}) {
  return (
    <div
      className={cn(
        compact
          ? "flex shrink-0 items-center"
          : "mt-2 flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center",
      )}
    >
      {actions.map((action, i) => (
        <ActionButton
          key={action.test_id ?? i}
          action={action}
          messageTaskId={taskId}
          onCompleted={onRecoveryRequested}
          compact={compact}
        />
      ))}
    </div>
  );
}

function ActionButton({
  action,
  messageTaskId,
  onCompleted,
  compact = false,
}: {
  action: MessageAction;
  messageTaskId?: string;
  onCompleted?: () => void;
  compact?: boolean;
}): ReactElement | null {
  const [state, setState] = useState<"idle" | "busy" | "done" | "error">("idle");
  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  const taskId = messageTaskId || activeTaskId;
  const store = useAppStoreApi();
  const archiveAndSwitch = useArchiveAndSwitchTask();
  const { removeTaskFromBoard } = useTaskRemoval({ store });

  const execute = useCallback(async () => {
    if (state === "busy") return;
    setState("busy");
    try {
      switch (action.type) {
        case "archive_task": {
          if (taskId) await archiveAndSwitch(taskId);
          break;
        }
        case "delete_task": {
          if (taskId) {
            const { activeTaskId, activeSessionId } = store.getState().tasks;
            await deleteTask(taskId);
            await removeTaskFromBoard(taskId, {
              wasActiveTaskId: activeTaskId,
              wasActiveSessionId: activeSessionId,
            });
          }
          break;
        }
        case "ws_request": {
          const client = getWebSocketClient();
          const params = action.params as
            | { method: string; payload: Record<string, unknown> }
            | undefined;
          if (!client || !params) throw new Error("WebSocket recovery request is unavailable");
          await client.request(params.method, params.payload);
          break;
        }
      }
      setState("done");
      if (action.type === "ws_request") onCompleted?.();
    } catch {
      setState("error");
      setTimeout(() => setState("idle"), 3000);
    }
  }, [action, state, taskId, store, archiveAndSwitch, removeTaskFromBoard]);

  // Once a ws_request has been fired, hide this button: it's no longer
  // actionable. If the recovery succeeds the whole ActionMessage unmounts via
  // isSessionActive; if it fails, a newer status/error message renders fresh
  // buttons, so this stale one would just confuse the user.
  if (state === "done" && action.type === "ws_request") return null;

  const Icon = action.icon ? ICON_MAP[action.icon] : null;
  const disabled = state === "busy" || state === "done";
  const isDestructive = action.variant === "destructive";

  const button = (
    <Button
      variant={compact ? "ghost" : "outline"}
      size="sm"
      className={cn(
        compact
          ? "h-auto min-h-11 shrink-0 px-2 text-xs cursor-pointer sm:min-h-8"
          : "h-auto min-h-11 w-full gap-1.5 text-xs cursor-pointer sm:min-h-8 sm:w-auto",
        isDestructive && "text-destructive hover:text-destructive",
      )}
      disabled={disabled}
      onClick={execute}
      data-testid={action.test_id}
    >
      {Icon && <Icon className="h-3 w-3" />}
      {action.label}
    </Button>
  );

  if (action.tooltip) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>{button}</TooltipTrigger>
        <TooltipContent side="top">{action.tooltip}</TooltipContent>
      </Tooltip>
    );
  }
  return button;
}
