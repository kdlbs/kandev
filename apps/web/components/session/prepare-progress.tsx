"use client";

import { useEffect, useRef, useState } from "react";
import {
  IconX,
  IconLoader2,
  IconAlertTriangle,
  IconTerminal2,
  IconFolder,
  IconGitBranch,
} from "@tabler/icons-react";
import type { TFunction } from "i18next";
import { Trans, useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { ExpandableRow } from "@/components/task/chat/messages/expandable-row";
import { isFallbackNoticeStep } from "@/lib/prepare/summarize";
import { cn } from "@/lib/utils";
import { stripAnsi } from "@/lib/utils/ansi";
import { isSetupScriptMessage } from "@/hooks/use-processed-messages";
import type { Message } from "@/lib/types/http";
import { AgentMcpPrepareActions } from "@/components/task/agent-mcp-prepare-actions";
import { StepIcon } from "./prepare-step-icon";
import { isAgentMcpAuthWarning } from "@/lib/prepare/agent-mcp-warning";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import type {
  PrepareStepInfo,
  SessionPrepareState,
} from "@/lib/state/slices/session-runtime/types";

const debug = createDebugLogger("chat:prepare-progress");

function formatStepDuration(startedAt?: string, endedAt?: string): string | null {
  if (!startedAt || !endedAt) return null;
  const ms = new Date(endedAt).getTime() - new Date(startedAt).getTime();
  if (ms < 0 || Number.isNaN(ms)) return null;
  if (ms < 1000) return `${ms}ms`;
  const secs = Math.round(ms / 1000);
  if (secs < 60) return `${secs}s`;
  const mins = Math.floor(secs / 60);
  const remSecs = secs % 60;
  return remSecs > 0 ? `${mins}m ${remSecs}s` : `${mins}m`;
}

type PrepareProgressProps = {
  sessionId: string;
};

type EffectiveStatus =
  | "preparing"
  | "failed"
  | "completed"
  | "completed_with_error"
  | "completed_with_warnings";

/** True when the command is short enough to display inline next to the step name. */
function isInlineCommand(cmd: string): boolean {
  return !cmd.includes("\n") && cmd.length <= 60;
}

function StepDetails({ step, blockCommand }: { step: PrepareStepInfo; blockCommand?: string }) {
  return (
    <div className="border-muted-foreground/20 mt-1 ml-0.5 border-l pl-3">
      {blockCommand && (
        <pre className="bg-muted/50 text-muted-foreground/60 mb-1 max-h-24 overflow-auto rounded px-2 py-1 font-mono text-[10px] whitespace-pre-wrap">
          {blockCommand}
        </pre>
      )}
      {step.output && (
        <pre className="text-muted-foreground/60 max-h-48 max-w-full overflow-y-auto whitespace-pre-wrap break-words text-xs">
          {stripAnsi(step.output)}
        </pre>
      )}
    </div>
  );
}

function StepWarning({ warning, warningDetail }: { warning: string; warningDetail?: string }) {
  const { t } = useTranslation();
  const [detailExpanded, setDetailExpanded] = useState(false);
  return (
    <div data-testid="prepare-warning-banner">
      <span className="text-amber-500 mt-0.5 block text-xs">{warning}</span>
      {warningDetail && (
        <>
          <button
            type="button"
            className="text-amber-500/70 hover:text-amber-500 mt-0.5 flex cursor-pointer items-center gap-0.5 text-xs"
            onClick={() => setDetailExpanded(!detailExpanded)}
          >
            {t("task:details")}
          </button>
          {detailExpanded && (
            <pre className="text-amber-500/60 mt-0.5 max-w-full whitespace-pre-wrap break-words text-xs">
              {warningDetail}
            </pre>
          )}
        </>
      )}
    </div>
  );
}

function StepMessages({ step }: { step: PrepareStepInfo }) {
  const { t } = useTranslation();
  const remoteHelperDownload = step.kind === "remote_helper_download";
  return (
    <>
      {step.warning && <StepWarning warning={step.warning} warningDetail={step.warningDetail} />}
      {remoteHelperDownload && step.status === "failed" && (
        <span className="text-muted-foreground mt-0.5 block text-xs">
          {t("task:remoteHelperDownloadRecovery")}
        </span>
      )}
      {step.error && (
        <pre className="text-destructive mt-0.5 max-w-full whitespace-pre-wrap break-words text-xs">
          {step.error}
        </pre>
      )}
    </>
  );
}

const AGENT_MCP_STEP_LABEL_KEYS: Record<string, string> = {
  agent_mcp_discovery: "task:agentMcpDiscoveryStep",
  agent_mcp_selection: "task:agentMcpSelectionStep",
  agent_mcp_credentials: "task:agentMcpCredentialsStep",
  agent_mcp_approval: "task:agentMcpApprovalStep",
  agent_mcp_verification: "task:agentMcpVerificationStep",
};

function agentMcpStepLabel(t: TFunction, step: PrepareStepInfo): string {
  const stage = t(AGENT_MCP_STEP_LABEL_KEYS[step.kind ?? ""] ?? "task:agentMcpPreparationStep");
  return step.mcpServerId
    ? t("task:agentMcpServerStep", { stage, server: step.mcpServerId })
    : stage;
}

function StepRowActions({
  isAgentMcp,
  step,
  sessionId,
  taskId,
}: {
  isAgentMcp: boolean;
  step: PrepareStepInfo;
  sessionId: string;
  taskId?: string;
}) {
  if (isAgentMcp) {
    if (step.status === "failed" && taskId) {
      return <AgentMcpPrepareActions step={step} sessionId={sessionId} taskId={taskId} />;
    }
    return null;
  }
  return <StepMessages step={step} />;
}

function resolveStepCommands(isAgentMcp: boolean, command?: string) {
  if (isAgentMcp || !command) {
    return { inlineCommand: undefined, blockCommand: undefined };
  }
  const isInline = isInlineCommand(command);
  return {
    inlineCommand: isInline ? command : undefined,
    blockCommand: !isInline ? command : undefined,
  };
}

function StepRow({
  step,
  sessionId,
  taskId,
}: {
  step: PrepareStepInfo;
  sessionId: string;
  taskId?: string;
}) {
  const { t } = useTranslation();
  const isAgentMcp = step.kind?.startsWith("agent_mcp_") === true;
  const isAuthWarning = isAgentMcpAuthWarning(step);
  const displayName = isAgentMcp
    ? agentMcpStepLabel(t, step)
    : remoteHelperDownloadStepLabel(t, step);
  const { inlineCommand, blockCommand } = resolveStepCommands(isAgentMcp, step.command);
  const hasExpandable = !isAgentMcp && (Boolean(step.output) || Boolean(blockCommand));
  const [detailsExpanded, setDetailsExpanded] = useState(
    step.status === "running" || step.status === "failed",
  );
  const duration = formatStepDuration(step.startedAt, step.endedAt);

  const nameClass = cn(
    "text-muted-foreground",
    step.status === "completed" && !step.warning && "line-through text-muted-foreground/60",
  );

  return (
    <div className="text-xs">
      <div className="flex min-w-0 flex-wrap items-center gap-2">
        <div className="flex-shrink-0">
          <StepIcon
            status={step.status}
            hasWarning={Boolean(step.warning)}
            isWarning={isAuthWarning}
          />
        </div>
        <span className={nameClass}>{displayName || t("task:preparingEllipsis")}</span>
        {inlineCommand && (
          <code className="text-muted-foreground/50 min-w-0 break-all font-mono text-[10px]">
            {inlineCommand}
          </code>
        )}
        {duration && <span className="text-muted-foreground/40 text-[10px]">{duration}</span>}
        {hasExpandable && (
          <button
            type="button"
            className="text-muted-foreground/40 hover:text-muted-foreground/70 cursor-pointer text-[10px]"
            onClick={() => setDetailsExpanded(!detailsExpanded)}
          >
            {detailsExpanded ? "[-]" : "[+]"}
          </button>
        )}
      </div>
      <div className="ml-[22px]">
        {detailsExpanded && !isAgentMcp && <StepDetails step={step} blockCommand={blockCommand} />}
        <StepRowActions isAgentMcp={isAgentMcp} step={step} sessionId={sessionId} taskId={taskId} />
      </div>
    </div>
  );
}

// deriveStatus maps the raw prepare state + ambient signals into the effective
// UI status. Kept small/pure so the hook stays a thin Zustand selector wrapper.
type DeriveStatusInput = {
  prepareStatus: string;
  sessionState: string | undefined;
  agentctlStatus: string | undefined;
  hasFailedStep: boolean;
  hasWarnings: boolean;
  hasRunningStep: boolean;
  hasPreparationAttempt?: boolean;
};

export function deriveStatus(input: DeriveStatusInput): EffectiveStatus {
  const {
    prepareStatus,
    sessionState,
    agentctlStatus,
    hasFailedStep,
    hasWarnings,
    hasRunningStep,
    hasPreparationAttempt,
  } = input;
  if (prepareStatus === "failed") return "failed";
  if (prepareStatus === "completed") {
    if (hasFailedStep) return "completed_with_error";
    if (hasWarnings) return "completed_with_warnings";
    return "completed";
  }
  // A step is still running — stay in preparing regardless of agentctl status.
  if (hasRunningStep) return "preparing";
  // prepareStatus === "preparing" from here on.
  // Agentctl ready implies preparation succeeded — treat as completed even if
  // the completed event hasn't arrived yet.
  if (!hasPreparationAttempt && agentctlStatus === "ready" && !hasFailedStep && !hasWarnings) {
    return "completed";
  }
  // If the session reached a terminal state but prepare is still "preparing",
  // treat it as failed — the completed event may have been lost.
  const isSessionTerminal =
    sessionState === "FAILED" || sessionState === "COMPLETED" || sessionState === "CANCELLED";
  return isSessionTerminal ? "failed" : "preparing";
}

function usePrepareStatus(sessionId: string) {
  const prepareState = useAppStore((state) => state.prepareProgress.bySessionId[sessionId] ?? null);
  const sessionState = useAppStore((state) => state.taskSessions.items[sessionId]?.state);
  const agentctlStatus = useAppStore(
    (state) => state.sessionAgentctl.itemsBySessionId[sessionId]?.status,
  );

  if (!prepareState) {
    // No live or hydrated prepare state. If the session has moved past the
    // initial state, preparation must have completed — show a minimal panel
    // so it doesn't vanish on refresh.
    const sessionStarted =
      sessionState && sessionState !== "CREATED" && sessionState !== "STARTING";
    if (sessionStarted) {
      return {
        status: "completed" as EffectiveStatus,
        prepareState: { sessionId, status: "completed", steps: [] } as SessionPrepareState,
      };
    }
    return { status: "preparing" as const, prepareState: null };
  }

  const status = deriveStatus({
    prepareStatus: prepareState.status,
    sessionState,
    agentctlStatus,
    hasFailedStep: prepareState.steps.some(
      (s) => s.status === "failed" && !isAgentMcpAuthWarning(s),
    ),
    hasWarnings: prepareState.steps.some((s) => Boolean(s.warning) || isAgentMcpAuthWarning(s)),
    hasRunningStep: prepareState.steps.some((s) => s.status === "running"),
    hasPreparationAttempt: Boolean(prepareState.preparationId || prepareState.preparationStartedAt),
  });
  return { status, prepareState };
}

// Tracks whether the panel is expanded. Starts from the status-derived default
// (`autoExpand`) and flips back to that default whenever the status transitions
// or the session changes — the "new context" rule. User's manual toggles stick
// within a single status window. Prev-value pattern (not useEffect) avoids a
// cascading re-render.
function useExpandedFlag(sessionId: string, autoExpand: boolean) {
  const [expanded, setExpanded] = useState(autoExpand);
  const [prevSessionId, setPrevSessionId] = useState(sessionId);
  const [prevAutoExpand, setPrevAutoExpand] = useState(autoExpand);
  if (sessionId !== prevSessionId) {
    setPrevSessionId(sessionId);
    setExpanded(autoExpand);
  } else if (autoExpand !== prevAutoExpand) {
    setPrevAutoExpand(autoExpand);
    setExpanded(autoExpand);
  }
  return [expanded, setExpanded] as const;
}

function HeaderIcon({ status }: { status: EffectiveStatus }) {
  if (status === "preparing") {
    return (
      <IconLoader2
        data-testid="prepare-progress-header-spinner"
        className="h-4 w-4 text-muted-foreground animate-spin"
      />
    );
  }
  if (status === "failed" || status === "completed_with_error") {
    return <IconX className="h-4 w-4 text-destructive" />;
  }
  if (status === "completed_with_warnings") {
    return <IconAlertTriangle className="h-4 w-4 text-amber-500" />;
  }
  return <IconTerminal2 className="h-4 w-4 text-muted-foreground" />;
}

function getHeaderLabel(
  t: TFunction,
  status: EffectiveStatus,
  prepareState: SessionPrepareState,
): string {
  if (status === "preparing") return t("task:preparingEnvironmentEllipsis");
  if (status === "failed") return t("task:environmentSetupFailed");
  if (status === "completed_with_error") {
    return t("task:environmentSetupFinishedWithErrors");
  }
  if (status === "completed_with_warnings") {
    const warningSteps = prepareState.steps.filter(
      (s) => Boolean(s.warning) || isAgentMcpAuthWarning(s),
    );
    if (warningSteps.length === 1 && isFallbackNoticeStep(warningSteps[0])) {
      return t("task:environmentPreparedOnFreshSandbox");
    }
    return t("task:environmentPreparedWithWarnings");
  }
  return t("task:environmentPrepared");
}

function InfoLine({ icon, children }: { icon: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="text-muted-foreground/70 flex items-center gap-2 text-xs">
      <div className="flex-shrink-0">{icon}</div>
      <span>{children}</span>
    </div>
  );
}

function Mono({ children }: { children: React.ReactNode }) {
  return <code className="text-muted-foreground font-mono text-[11px]">{children}</code>;
}

function hasStepDetails(step: PrepareStepInfo): boolean {
  return Boolean(step.command || step.output || step.error || step.warning || step.warningDetail);
}

function isVisibleStep(step: PrepareStepInfo): boolean {
  if (step.status === "skipped" && !hasStepDetails(step)) return false;
  if (step.kind === "remote_helper_download") return true;
  if (step.kind?.startsWith("agent_mcp_")) return true;
  return step.name.trim() !== "" || hasStepDetails(step);
}

function remoteHelperDownloadStepLabel(t: TFunction, step: PrepareStepInfo): string {
  if (step.kind !== "remote_helper_download") return step.name;
  const options = { platform: step.remotePlatform ?? "" };
  if (step.status === "running") return t("task:remoteHelperDownloadRunning", options);
  if (step.status === "completed") return t("task:remoteHelperDownloadCompleted", options);
  if (step.status === "failed" && step.failureCode === "timeout") {
    return t("task:remoteHelperDownloadTimedOut", options);
  }
  if (step.status === "failed") return t("task:remoteHelperDownloadFailed", options);
  return t("task:preparingEllipsis");
}

type ScriptStatus = "starting" | "running" | "exited" | "failed";

// Map the script_execution status vocabulary (starting/running/exited/failed)
// onto the PrepareStep status vocabulary (running/completed/failed).
function scriptStatusToStepStatus(
  status: ScriptStatus | undefined,
  exitCode: number | undefined,
): string {
  if (status === "starting" || status === "running") return "running";
  if (status === "failed") return "failed";
  if (status === "exited") {
    if (exitCode === undefined || exitCode === 0) return "completed";
    return "failed";
  }
  return status ?? "running";
}

type ScriptMetadata = {
  script_type?: string;
  command?: string;
  status?: ScriptStatus;
  exit_code?: number;
  error?: string;
  started_at?: string;
  completed_at?: string;
};

// Convert a setup-script `script_execution` message into the same step shape
// the existing prepare panel renders, so it slots in alongside Validate/Sync/
// Create-worktree without a parallel render path.
function setupScriptMessageToStep(t: TFunction, message: Message): PrepareStepInfo {
  const meta = (message.metadata ?? {}) as ScriptMetadata;
  const command = meta.command ?? "";
  const status = scriptStatusToStepStatus(meta.status, meta.exit_code);
  const stepError =
    status === "failed"
      ? meta.error ||
        (meta.exit_code !== undefined && meta.exit_code !== 0
          ? t("task:scriptExitedWithCode", { code: meta.exit_code })
          : t("task:scriptFailed"))
      : undefined;
  return {
    name: t("task:runRepositorySetupScript"),
    command,
    status,
    output: message.content || undefined,
    error: stepError,
    startedAt: meta.started_at,
    endedAt: meta.completed_at,
  };
}

function useSetupScriptSteps(sessionId: string): PrepareStepInfo[] {
  const { t } = useTranslation();
  const messages = useAppStore((state) => state.messages.bySession[sessionId]);
  if (!messages || messages.length === 0) return [];
  return messages.filter(isSetupScriptMessage).map((m) => setupScriptMessageToStep(t, m));
}

function SessionInfo({ sessionId }: { sessionId: string }) {
  const session = useAppStore((state) => state.taskSessions.items[sessionId]);
  if (!session) return null;

  const { worktree_id, worktree_path, worktree_branch, base_branch } = session;
  const isWorktree = Boolean(worktree_id);

  // Nothing useful to show for sessions without workspace info.
  if (!worktree_path && !worktree_branch) return null;

  return (
    <div className="border-muted mt-2 space-y-1 border-t pt-2">
      {isWorktree && worktree_path && (
        <InfoLine icon={<IconFolder className="h-3 w-3" />}>
          <Trans i18nKey="task:isolatedWorktreeAt" values={{ path: worktree_path }}>
            Isolated worktree at <Mono>{worktree_path}</Mono>
          </Trans>
        </InfoLine>
      )}
      {!isWorktree && worktree_path && (
        <InfoLine icon={<IconFolder className="h-3 w-3" />}>
          <Trans i18nKey="task:workingIn" values={{ path: worktree_path }}>
            Working in <Mono>{worktree_path}</Mono>
          </Trans>
        </InfoLine>
      )}
      {worktree_branch && (
        <InfoLine icon={<IconGitBranch className="h-3 w-3" />}>
          {base_branch ? (
            <Trans
              i18nKey="task:branchBasedOn"
              values={{ branch: worktree_branch, base: base_branch }}
            >
              Branch <Mono>{worktree_branch}</Mono>, based on <Mono>{base_branch}</Mono>
            </Trans>
          ) : (
            <Trans i18nKey="task:onBranch" values={{ branch: worktree_branch }}>
              On branch <Mono>{worktree_branch}</Mono>
            </Trans>
          )}
        </InfoLine>
      )}
    </div>
  );
}

type PrepareSnapshot = {
  status: string;
  autoExpand: boolean;
  expanded: boolean;
  visibleSteps: number;
  hasPrepareState: boolean;
};

function prepareSnapshotChanged(prev: PrepareSnapshot | null, next: PrepareSnapshot): boolean {
  if (!prev) return true;
  return (
    prev.status !== next.status ||
    prev.autoExpand !== next.autoExpand ||
    prev.expanded !== next.expanded ||
    prev.visibleSteps !== next.visibleSteps ||
    prev.hasPrepareState !== next.hasPrepareState
  );
}

function logPrepareSnapshotChange(
  prev: PrepareSnapshot | null,
  next: PrepareSnapshot,
  sessionId: string,
  rawPrepareStatus: string,
  sessionState: string,
) {
  debug(prev ? "transition" : "init", {
    sessionId,
    ...next,
    prevStatus: prev?.status ?? "-",
    prevAutoExpand: prev?.autoExpand ?? null,
    prevExpanded: prev?.expanded ?? null,
    prevVisibleSteps: prev?.visibleSteps ?? -1,
    rawPrepareStatus,
    sessionState,
  });
}

function hasPrepareDetails(
  visibleStepCount: number,
  hasSessionInfo: boolean,
  hasFailureDetails: boolean,
): boolean {
  return visibleStepCount > 0 || hasSessionInfo || hasFailureDetails;
}

export function PrepareProgress({ sessionId }: PrepareProgressProps) {
  const { t } = useTranslation();
  const { status, prepareState } = usePrepareStatus(sessionId);
  const session = useAppStore((state) => state.taskSessions.items[sessionId]);
  const setupScriptSteps = useSetupScriptSteps(sessionId);
  // Keep active progress and warnings visible. Failure diagnostics remain
  // secondary until the user explicitly asks to inspect them.
  const autoExpand = status === "preparing" || status === "completed_with_warnings";
  const [expanded, setExpanded] = useExpandedFlag(sessionId, autoExpand);

  // Track status/expand transitions over the panel's lifetime — the
  // remote-executor bug suspects the panel staying expanded (tall) while the
  // env is still "preparing" combined with an initial-scroll race.
  const prevSnapshotRef = useRef<PrepareSnapshot | null>(null);
  useEffect(() => {
    if (!isDebug()) return;
    const snapshot: PrepareSnapshot = {
      status,
      autoExpand,
      expanded,
      visibleSteps: prepareState ? prepareState.steps.filter(isVisibleStep).length : 0,
      hasPrepareState: Boolean(prepareState),
    };
    const prev = prevSnapshotRef.current;
    if (!prepareSnapshotChanged(prev, snapshot)) return;
    logPrepareSnapshotChange(
      prev,
      snapshot,
      sessionId,
      prepareState?.status ?? "-",
      session?.state ?? "-",
    );
    prevSnapshotRef.current = snapshot;
  }, [sessionId, status, autoExpand, expanded, prepareState, session?.state]);

  if (!prepareState) return null;

  const hasSessionInfo = Boolean(session?.worktree_path || session?.worktree_branch);
  // Per-repo setup scripts run inside worktree.Manager.Create, so visually they
  // belong after Create-worktree finishes. Appending keeps the existing prepare
  // step order intact.
  const visibleSteps = [...prepareState.steps.filter(isVisibleStep), ...setupScriptSteps];
  const headerLabel = getHeaderLabel(t, status, prepareState);
  const isErrorStatus = status === "failed" || status === "completed_with_error";
  const headerClass = cn("text-xs", isErrorStatus ? "text-destructive" : "text-muted-foreground");
  const hasFailureDetails = isErrorStatus && Boolean(prepareState.errorMessage);
  const hasExpandableContent = hasPrepareDetails(
    visibleSteps.length,
    hasSessionInfo,
    hasFailureDetails,
  );

  return (
    <div
      data-testid="prepare-progress-panel"
      data-status={status}
      data-expanded={expanded}
      className="min-w-0 max-w-full"
    >
      <ExpandableRow
        icon={<HeaderIcon status={status} />}
        header={
          <div className="flex min-w-0 flex-wrap items-center justify-between gap-2">
            <span
              data-testid="prepare-progress-toggle"
              className={cn(headerClass, "min-w-0 break-words font-medium")}
            >
              {headerLabel}
            </span>
            {hasExpandableContent && (
              <button
                type="button"
                aria-expanded={expanded}
                aria-label={
                  expanded ? t("task:hidePreparationDetails") : t("task:showPreparationDetails")
                }
                className="min-h-11 cursor-pointer text-xs text-muted-foreground underline-offset-4 hover:underline sm:min-h-0"
                onClick={(event) => {
                  event.stopPropagation();
                  setExpanded(!expanded);
                }}
              >
                {expanded ? t("task:hideDetails") : t("task:showDetails")}
              </button>
            )}
          </div>
        }
        hasExpandableContent={hasExpandableContent}
        isExpanded={expanded}
        onToggle={() => setExpanded(!expanded)}
      >
        {hasFailureDetails && (
          <pre className="mb-2 max-h-48 max-w-full overflow-y-auto whitespace-pre-wrap break-words rounded bg-muted/40 p-2 text-xs text-muted-foreground">
            {prepareState.errorMessage}
          </pre>
        )}
        <div className="space-y-1">
          {visibleSteps.map((step, i) => (
            <StepRow key={i} step={step} sessionId={sessionId} taskId={session?.task_id} />
          ))}
        </div>
        <SessionInfo sessionId={sessionId} />
      </ExpandableRow>
    </div>
  );
}
