"use client";

import { useTranslation } from "react-i18next";
import {
  IconAlertTriangle,
  IconChevronDown,
  IconLoader2,
  IconPointFilled,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { Progress } from "@kandev/ui/progress";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { cn } from "@kandev/ui/lib/utils";
import { formatLspElapsed, LSP_LONG_INITIALIZATION_MS } from "@/lib/lsp/lsp-progress-view";
import type { TaskLspLanguageView, TaskLspVisualState } from "@/lib/lsp/task-lsp-view-model";
import type { TaskLspAction, TaskLspPolicy } from "@/lib/types/http-lsp";

const STATE_KEYS: Record<TaskLspVisualState, string> = {
  error: "lsp:taskStateError",
  server_work: "lsp:taskStateServerWork",
  initializing: "lsp:taskStateInitializing",
  installing: "lsp:taskStateInstalling",
  starting: "lsp:taskStateStarting",
  queued: "lsp:taskStateQueued",
  stopping: "lsp:taskStateStopping",
  ready: "lsp:taskStateReady",
  unsupported: "lsp:taskStateUnsupported",
  waiting: "lsp:taskStateWaiting",
  detected: "lsp:taskStateDetected",
  configured: "lsp:taskStateConfigured",
  stopped: "lsp:taskStateStopped",
  off: "lsp:taskStateOff",
};

const POLICY_KEYS: Record<TaskLspPolicy, string> = {
  inherit: "lsp:policyInherit",
  keep_warm: "lsp:policyKeepWarm",
  disabled: "lsp:policyDisabled",
};

const ACTION_KEYS: Partial<Record<TaskLspAction, string>> = {
  start: "lsp:lastActionStarted",
  stop: "lsp:lastActionStopped",
  restart: "lsp:lastActionRestarted",
  set_policy: "lsp:lastActionPolicyChanged",
  reconcile: "lsp:lastActionReconciled",
};

const CONTROL_KEYS = {
  start: "lsp:start",
  stop: "lsp:stop",
  restart: "lsp:restart",
} as const;

const INITIATOR_KEYS = {
  user: "lsp:initiatorUser",
  agent: "lsp:initiatorAgent",
  automatic: "lsp:initiatorAutomatic",
} as const;

const REASON_KEYS: Record<string, string> = {
  user_start: "lsp:reasonUserStart",
  user_stop: "lsp:reasonUserStop",
  user_restart: "lsp:reasonUserRestart",
  user_set_policy: "lsp:reasonPolicyChange",
  task_archived: "lsp:reasonTaskArchived",
  task_deleted: "lsp:reasonTaskDeleted",
  reconcile_missing_runtime: "lsp:reasonMissingRuntime",
  workspace_roots_changed: "lsp:reasonWorkspaceRootsChanged",
};

type TaskLspLanguageRowProps = {
  row: TaskLspLanguageView;
  now: number;
  touch: boolean;
  open: boolean;
  pending?: TaskLspAction;
  onOpenChange: (open: boolean) => void;
  onStart: () => void;
  onStop: () => void;
  onRestart: () => void;
  onSetPolicy: (policy: TaskLspPolicy) => void;
};

function stateTone(state: TaskLspVisualState): string {
  if (state === "error") return "text-destructive";
  if (state === "server_work" || state === "initializing") return "text-blue-500";
  if (state === "ready") return "text-emerald-500";
  if (state === "unsupported") return "text-amber-500";
  return "text-muted-foreground";
}

function reasonLabel(
  reason: string,
  t: (key: string, values?: Record<string, unknown>) => string,
): string {
  const key = REASON_KEYS[reason];
  return key ? t(key) : t("lsp:reasonOther", { reason });
}

function DetectionEvidence({ row }: { row: TaskLspLanguageView }) {
  const { t } = useTranslation();
  const snapshot = row.snapshot;
  let key = "lsp:detectionNotDetected";
  if (snapshot.detected) key = "lsp:detectionDetected";
  else if (snapshot.detection_state === "scanning") key = "lsp:detectionScanning";
  else if (snapshot.detection_state === "partial") key = "lsp:detectionPartial";
  else if (snapshot.detection_state === "unavailable") key = "lsp:detectionUnavailable";
  return (
    <span>
      {t(key)}
      {snapshot.detection_truncated ? ` · ${t("lsp:detectionTruncated")}` : ""}
    </span>
  );
}

function ProgressEvidence({ row }: { row: TaskLspLanguageView }) {
  const { t } = useTranslation();
  if (!row.work) return null;
  return (
    <div
      className="min-w-0 space-y-1.5 rounded-md border border-blue-500/20 bg-blue-500/5 p-2.5 [overflow-wrap:anywhere]"
      data-testid="task-lsp-progress"
      data-lsp-progress-kind="active"
      aria-live="polite"
    >
      <div className="flex min-w-0 items-start gap-2">
        <IconLoader2 className="mt-0.5 size-3.5 shrink-0 animate-spin text-blue-500" aria-hidden />
        <div className="min-w-0">
          <p className="font-medium text-foreground">{row.work.title}</p>
          {row.work.message ? <p className="text-muted-foreground">{row.work.message}</p> : null}
        </div>
      </div>
      {row.work.percentage !== undefined ? (
        <div className="flex items-center gap-2">
          <Progress
            value={row.work.percentage}
            className="min-w-0 flex-1"
            data-testid="lsp-work-progress-bar"
            aria-label={t("lsp:progressPercentage", {
              title: row.work.title,
              percentage: row.work.percentage,
            })}
          />
          <span className="shrink-0 text-xs tabular-nums">{row.work.percentage}%</span>
        </div>
      ) : (
        <p className="text-muted-foreground">{t("lsp:noPercentage")}</p>
      )}
      {row.elapsedMs !== null ? (
        <p className="text-muted-foreground tabular-nums">
          {t("lsp:elapsed", { elapsed: formatLspElapsed(row.elapsedMs) })}
        </p>
      ) : null}
      <p className="text-amber-700 dark:text-amber-300">
        {t("lsp:crossFileDefinitionsMayBeIncomplete")}
      </p>
    </div>
  );
}

function InitializationEvidence({ row, now }: { row: TaskLspLanguageView; now: number }) {
  const { t } = useTranslation();
  const snapshot = row.snapshot;
  if (row.work || (snapshot.phase !== "process_started" && snapshot.phase !== "initializing")) {
    return null;
  }
  const startedAt = Date.parse(snapshot.initialize_started_at ?? snapshot.process_started_at ?? "");
  const elapsedMs = Number.isFinite(startedAt) ? Math.max(0, now - startedAt) : 0;
  const longRunning = elapsedMs >= LSP_LONG_INITIALIZATION_MS;
  let guidance = t("lsp:crossFileFeaturesAfterInitialization");
  if (longRunning) {
    guidance = t(
      row.language === "kotlin"
        ? "lsp:kotlinInitializationGuidance"
        : "lsp:crossFileFeaturesUnavailable",
    );
  }
  return (
    <div
      className="min-w-0 space-y-1.5 rounded-md border border-blue-500/20 bg-blue-500/5 p-2.5 text-xs [overflow-wrap:anywhere]"
      data-testid="task-lsp-initialization"
      data-lsp-initialization-stage={longRunning ? "long_running" : "initialize_pending"}
      aria-live="polite"
    >
      <div className="flex items-start gap-2">
        {longRunning ? (
          <IconAlertTriangle className="mt-0.5 size-3.5 shrink-0 text-amber-500" aria-hidden />
        ) : (
          <IconLoader2
            className="mt-0.5 size-3.5 shrink-0 animate-spin text-blue-500"
            aria-hidden
          />
        )}
        <div className="min-w-0 space-y-1">
          <p className="font-medium text-foreground">
            {t(longRunning ? "lsp:initializationLongRunning" : "lsp:serverProcessStarted")}
          </p>
          <p className="text-muted-foreground">
            {t(longRunning ? "lsp:serverStillInitializing" : "lsp:waitingForInitializeResponse")}
          </p>
        </div>
      </div>
      <p className="text-muted-foreground tabular-nums">
        {t("lsp:elapsed", { elapsed: formatLspElapsed(elapsedMs) })}
      </p>
      <p className={longRunning ? "text-amber-700 dark:text-amber-300" : "text-muted-foreground"}>
        {guidance}
      </p>
    </div>
  );
}

function CompletedWorkEvidence({ row }: { row: TaskLspLanguageView }) {
  const { t } = useTranslation();
  const completed = row.snapshot.last_completed_work;
  if (row.work || !completed) return null;
  return (
    <div
      className="min-w-0 space-y-1 rounded-md border border-emerald-500/20 bg-emerald-500/5 p-2.5 text-xs [overflow-wrap:anywhere]"
      data-testid="task-lsp-completed-work"
      data-lsp-progress-kind="completed"
    >
      <p className="font-medium text-foreground">{t("lsp:serverWorkFinished")}</p>
      {completed.message ? <p className="text-muted-foreground">{completed.message}</p> : null}
      <p className="text-muted-foreground">{t("lsp:reportedWorkDisclaimer")}</p>
    </div>
  );
}

function IdleEvidence({ row }: { row: TaskLspLanguageView }) {
  const { t } = useTranslation();
  if (row.work || row.snapshot.last_completed_work || row.snapshot.phase !== "ready") return null;
  return (
    <div
      className="min-w-0 space-y-1 rounded-md border p-2.5 text-xs [overflow-wrap:anywhere]"
      data-testid="task-lsp-idle"
      data-lsp-progress-kind="idle"
    >
      <p className="font-medium text-foreground">{t("lsp:noBackgroundWorkReported")}</p>
      <p className="text-muted-foreground">{t("lsp:noBackgroundWorkReportedDescription")}</p>
    </div>
  );
}

function StateEvidence({ row }: { row: TaskLspLanguageView }) {
  const { t } = useTranslation();
  if (row.snapshot.phase === "queued") {
    return (
      <div
        className="rounded-md border border-amber-500/25 bg-amber-500/5 p-2.5 text-xs text-amber-700 dark:text-amber-300"
        data-testid="task-lsp-capacity-wait"
        role="status"
      >
        {t("lsp:waitingForCapacity")}
      </div>
    );
  }
  if (row.snapshot.phase === "unsupported") {
    return (
      <div
        className="rounded-md border border-amber-500/25 bg-amber-500/5 p-2.5 text-xs text-amber-700 dark:text-amber-300"
        data-testid="task-lsp-unsupported"
        role="alert"
      >
        {t("lsp:taskExecutorUnsupported")}
      </div>
    );
  }
  return null;
}

function ErrorEvidence({ row }: { row: TaskLspLanguageView }) {
  const { t } = useTranslation();
  const snapshot = row.snapshot;
  if (snapshot.phase !== "error") return null;
  const rawMessage = snapshot.error_message ?? "";
  const missingBinary =
    snapshot.error_code === "binary_unavailable" ||
    rawMessage.toLocaleLowerCase().includes(`${row.language}-lsp not found`);
  const processExited =
    snapshot.error_code === "process_exited" || rawMessage.trim().toLocaleLowerCase() === "eof";
  let message = rawMessage || snapshot.error_code || t("lsp:languageServerError");
  if (missingBinary) message = t("lsp:languageServerNotFound");
  else if (processExited) message = t("lsp:languageServerExited");
  return (
    <div
      role="alert"
      className="min-w-0 space-y-1 rounded-md border border-destructive/25 bg-destructive/5 p-2.5 text-xs text-destructive [overflow-wrap:anywhere]"
    >
      <div className="flex min-w-0 gap-2">
        <IconAlertTriangle className="mt-0.5 size-3.5 shrink-0" aria-hidden />
        <span>{message}</span>
      </div>
      {missingBinary ? (
        <p className="pl-5">
          {t(
            row.language === "kotlin"
              ? "lsp:installKotlinLsp"
              : "lsp:installLanguageServerManually",
          )}
        </p>
      ) : null}
    </div>
  );
}

function LifecycleEvidence({ row, now }: { row: TaskLspLanguageView; now: number }) {
  const { t } = useTranslation();
  const snapshot = row.snapshot;
  const processStartedAt = snapshot.process_started_at
    ? Date.parse(snapshot.process_started_at)
    : Number.NaN;
  const actionKey = ACTION_KEYS[snapshot.last_action];
  return (
    <div className="grid gap-1 text-xs text-muted-foreground sm:grid-cols-2">
      {snapshot.generation > 0 ? (
        <span>{t("lsp:generation", { generation: snapshot.generation })}</span>
      ) : null}
      {Number.isFinite(processStartedAt) ? (
        <span className="tabular-nums">
          {t("lsp:startedAgo", { elapsed: formatLspElapsed(Math.max(0, now - processStartedAt)) })}
        </span>
      ) : null}
      {actionKey ? (
        <span>
          {t("lsp:lastActionBy", {
            action: t(actionKey),
            initiator: t(INITIATOR_KEYS[snapshot.last_initiator]),
          })}
        </span>
      ) : null}
      {snapshot.last_restart_reason ? (
        <span>
          {t("lsp:lastRestartReason", { reason: reasonLabel(snapshot.last_restart_reason, t) })}
        </span>
      ) : null}
      {snapshot.last_stop_reason ? (
        <span>
          {t("lsp:lastStopReason", { reason: reasonLabel(snapshot.last_stop_reason, t) })}
        </span>
      ) : null}
    </div>
  );
}

function LanguageHeader({ row, open }: { row: TaskLspLanguageView; open: boolean }) {
  const { t } = useTranslation();
  return (
    <CollapsibleTrigger
      className="group flex min-h-11 w-full cursor-pointer items-start gap-3 px-3 py-2.5 text-left outline-none transition-colors hover:bg-muted/40 focus-visible:bg-muted/40 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
      aria-label={t(open ? "lsp:collapseLanguageDetails" : "lsp:expandLanguageDetails", {
        language: row.label,
      })}
      data-testid={`task-lsp-language-trigger-${row.language}`}
    >
      <div className="min-w-0">
        <div className="flex items-center gap-1.5">
          <IconPointFilled className={cn("size-4 shrink-0", stateTone(row.state))} aria-hidden />
          <h3 className="truncate font-semibold">{row.label}</h3>
        </div>
        <p className={cn("text-xs", stateTone(row.state))}>{t(STATE_KEYS[row.state])}</p>
      </div>
      <div className="ml-auto flex shrink-0 items-center gap-2 text-right text-xs text-muted-foreground">
        <span className="max-w-40 [overflow-wrap:anywhere]">
          <DetectionEvidence row={row} />
        </span>
        <IconChevronDown
          className="size-4 shrink-0 transition-transform group-data-[state=open]:rotate-180"
          aria-hidden
        />
      </div>
    </CollapsibleTrigger>
  );
}

function PolicySection({
  row,
  disabled,
  controlHeight,
  touch,
  onSetPolicy,
}: Pick<TaskLspLanguageRowProps, "row" | "onSetPolicy"> & {
  disabled: boolean;
  controlHeight: string;
  touch: boolean;
}) {
  const { t } = useTranslation();
  const snapshot = row.snapshot;
  return (
    <>
      <div className="flex min-w-0 items-center justify-between gap-3 text-xs">
        <span className="text-muted-foreground">{t("lsp:taskPolicy")}</span>
        <Select
          value={snapshot.policy}
          disabled={disabled}
          onValueChange={(value) => onSetPolicy(value as TaskLspPolicy)}
        >
          <SelectTrigger
            aria-label={t("lsp:taskPolicyAria", { language: row.label })}
            className={cn("w-44 max-w-[65%] justify-between", controlHeight)}
            data-testid={`task-lsp-policy-${row.language}`}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent position="popper" align="end">
            {(["inherit", "keep_warm", "disabled"] as const).map((policy) => (
              <SelectItem key={policy} value={policy} className={touch ? "min-h-11" : undefined}>
                {t(POLICY_KEYS[policy])}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <p className="text-xs text-muted-foreground">
        {t("lsp:effectivePolicy", { policy: t(POLICY_KEYS[snapshot.effective_policy]) })}
      </p>
    </>
  );
}

function RestartRequiredEvidence({ row }: { row: TaskLspLanguageView }) {
  const { t } = useTranslation();
  const snapshot = row.snapshot;
  if (!snapshot.restart_required) return null;
  const reason = snapshot.restart_required_reason
    ? reasonLabel(snapshot.restart_required_reason, t)
    : t("lsp:reasonWorkspaceChanged");
  return (
    <div className="rounded-md border border-amber-500/25 bg-amber-500/5 p-2.5 text-xs text-amber-700 dark:text-amber-300">
      {t("lsp:restartRequired", { reason })}
    </div>
  );
}

function LifecycleButtons({
  row,
  disabled,
  controlHeight,
  onStart,
  onStop,
  onRestart,
}: Pick<TaskLspLanguageRowProps, "row" | "onStart" | "onStop" | "onRestart"> & {
  disabled: boolean;
  controlHeight: string;
}) {
  const { t } = useTranslation();
  const buttons = [
    { action: "start", enabled: row.actions.start, variant: "default", onClick: onStart },
    { action: "stop", enabled: row.actions.stop, variant: "outline", onClick: onStop },
    { action: "restart", enabled: row.actions.restart, variant: "outline", onClick: onRestart },
  ] as const;
  return (
    <div className="grid grid-cols-3 gap-2">
      {buttons.map((button) => (
        <Button
          key={button.action}
          type="button"
          variant={button.variant}
          size="sm"
          className={cn("cursor-pointer px-2", controlHeight)}
          disabled={disabled || !button.enabled}
          onClick={button.onClick}
          data-testid="lsp-lifecycle-action"
          data-lsp-action={button.action}
        >
          {t(CONTROL_KEYS[button.action])}
        </Button>
      ))}
    </div>
  );
}

export function TaskLspLanguageRow({
  row,
  now,
  touch,
  open,
  pending,
  onOpenChange,
  onStart,
  onStop,
  onRestart,
  onSetPolicy,
}: TaskLspLanguageRowProps) {
  const disabled = pending !== undefined;
  const snapshot = row.snapshot;
  const controlHeight = touch ? "h-11" : "h-8";
  return (
    <Collapsible open={open} onOpenChange={onOpenChange} asChild>
      <article
        className="min-w-0 overflow-hidden rounded-lg border bg-card shadow-sm [overflow-wrap:anywhere]"
        data-testid={`task-lsp-language-${row.language}`}
        data-lsp-state={row.state}
        data-lsp-policy={snapshot.policy}
        data-lsp-generation={snapshot.generation}
      >
        <LanguageHeader row={row} open={open} />
        <CollapsibleContent>
          <div className="min-w-0 space-y-3 border-t bg-background/40 p-3">
            <PolicySection
              row={row}
              disabled={disabled}
              controlHeight={controlHeight}
              touch={touch}
              onSetPolicy={onSetPolicy}
            />

            <ProgressEvidence row={row} />
            <InitializationEvidence row={row} now={now} />
            <CompletedWorkEvidence row={row} />
            <IdleEvidence row={row} />
            <StateEvidence row={row} />
            <ErrorEvidence row={row} />
            <RestartRequiredEvidence row={row} />
            <LifecycleEvidence row={row} now={now} />
            <LifecycleButtons
              row={row}
              disabled={disabled}
              controlHeight={controlHeight}
              onStart={onStart}
              onStop={onStop}
              onRestart={onRestart}
            />
          </div>
        </CollapsibleContent>
      </article>
    </Collapsible>
  );
}
