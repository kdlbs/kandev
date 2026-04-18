"use client";

import { useState } from "react";
import {
  IconCheck,
  IconX,
  IconLoader2,
  IconAlertTriangle,
  IconTerminal2,
  IconFolder,
  IconGitBranch,
} from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { ExpandableRow } from "@/components/task/chat/messages/expandable-row";
import { cn } from "@/lib/utils";
import type {
  PrepareStepInfo,
  SessionPrepareState,
} from "@/lib/state/slices/session-runtime/types";

const ANSI_RE = /\x1b\[[0-9;]*[a-zA-Z]/g;
function stripAnsi(s: string): string {
  return s.replace(ANSI_RE, "");
}

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

function StepIcon({ status, hasWarning }: { status: string; hasWarning?: boolean }) {
  if (status === "completed" && hasWarning) {
    return <IconAlertTriangle className="h-3.5 w-3.5 text-amber-500" />;
  }
  if (status === "completed") {
    return <IconCheck className="h-3.5 w-3.5 text-green-500" />;
  }
  if (status === "failed") {
    return <IconX className="h-3.5 w-3.5 text-destructive" />;
  }
  if (status === "running") {
    return <IconLoader2 className="h-3.5 w-3.5 text-muted-foreground animate-spin" />;
  }
  return <div className="h-3.5 w-3.5 rounded-full border border-muted-foreground/30" />;
}

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
        <pre className="text-muted-foreground/60 max-h-48 overflow-auto whitespace-pre text-xs">
          {stripAnsi(step.output)}
        </pre>
      )}
    </div>
  );
}

function StepWarning({ warning, warningDetail }: { warning: string; warningDetail?: string }) {
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
            Details
          </button>
          {detailExpanded && (
            <pre className="text-amber-500/60 mt-0.5 overflow-x-auto whitespace-pre text-xs">
              {warningDetail}
            </pre>
          )}
        </>
      )}
    </div>
  );
}

function StepMessages({ step }: { step: PrepareStepInfo }) {
  return (
    <>
      {step.warning && <StepWarning warning={step.warning} warningDetail={step.warningDetail} />}
      {step.error && (
        <pre className="text-destructive mt-0.5 overflow-x-auto whitespace-pre text-xs">
          {step.error}
        </pre>
      )}
    </>
  );
}

function StepRow({ step }: { step: PrepareStepInfo }) {
  const inlineCommand = step.command && isInlineCommand(step.command) ? step.command : undefined;
  const blockCommand = step.command && !isInlineCommand(step.command) ? step.command : undefined;
  const hasExpandable = Boolean(step.output) || Boolean(blockCommand);
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
      <div className="flex items-center gap-2">
        <div className="flex-shrink-0">
          <StepIcon status={step.status} hasWarning={Boolean(step.warning)} />
        </div>
        <span className={nameClass}>{step.name || "Preparing..."}</span>
        {inlineCommand && (
          <code className="text-muted-foreground/50 font-mono text-[10px]">{inlineCommand}</code>
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
      {/* Content below the step header, indented past the icon */}
      <div className="ml-[22px]">
        {detailsExpanded && <StepDetails step={step} blockCommand={blockCommand} />}
        <StepMessages step={step} />
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
};

function deriveStatus(input: DeriveStatusInput): EffectiveStatus {
  const {
    prepareStatus,
    sessionState,
    agentctlStatus,
    hasFailedStep,
    hasWarnings,
    hasRunningStep,
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
  if (agentctlStatus === "ready" && !hasFailedStep && !hasWarnings) return "completed";
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
    hasFailedStep: prepareState.steps.some((s) => s.status === "failed"),
    hasWarnings: prepareState.steps.some((s) => s.warning),
    hasRunningStep: prepareState.steps.some((s) => s.status === "running"),
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

function getHeaderLabel(status: EffectiveStatus, prepareState: SessionPrepareState): string {
  if (status === "preparing") return "Preparing environment...";
  if (status === "failed") return prepareState.errorMessage || "Environment preparation failed";
  if (status === "completed_with_error") {
    const failed = prepareState.steps.find((s) => s.status === "failed");
    // `||` (not `??`) so an empty-string error also falls through to the name fallback.
    return failed?.error || `${failed?.name ?? "Environment prepared"} — step failed`;
  }
  if (status === "completed_with_warnings") return "Environment prepared with warnings";
  return "Environment prepared";
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
          Isolated worktree at <Mono>{worktree_path}</Mono>
        </InfoLine>
      )}
      {!isWorktree && worktree_path && (
        <InfoLine icon={<IconFolder className="h-3 w-3" />}>
          Working in <Mono>{worktree_path}</Mono>
        </InfoLine>
      )}
      {worktree_branch && (
        <InfoLine icon={<IconGitBranch className="h-3 w-3" />}>
          {base_branch ? (
            <>
              Branch <Mono>{worktree_branch}</Mono>, based on <Mono>{base_branch}</Mono>
            </>
          ) : (
            <>
              On branch <Mono>{worktree_branch}</Mono>
            </>
          )}
        </InfoLine>
      )}
    </div>
  );
}

export function PrepareProgress({ sessionId }: PrepareProgressProps) {
  const { status, prepareState } = usePrepareStatus(sessionId);
  const session = useAppStore((state) => state.taskSessions.items[sessionId]);
  // Only the clean-success status auto-collapses. Anything with running/error/
  // warning context stays open by default so the user sees what's happening.
  const autoExpand = status !== "completed";
  const [expanded, setExpanded] = useExpandedFlag(sessionId, autoExpand);

  if (!prepareState) return null;

  const hasSessionInfo = Boolean(session?.worktree_path || session?.worktree_branch);
  const visibleSteps = prepareState.steps.filter(
    (step) =>
      step.name.trim() !== "" ||
      Boolean(step.output) ||
      Boolean(step.error) ||
      Boolean(step.warning),
  );
  const headerLabel = getHeaderLabel(status, prepareState);
  const isErrorStatus = status === "failed" || status === "completed_with_error";
  const headerClass = cn("text-xs", isErrorStatus ? "text-destructive" : "text-muted-foreground");

  return (
    <div data-testid="prepare-progress-panel" data-status={status} data-expanded={expanded}>
      <ExpandableRow
        icon={<HeaderIcon status={status} />}
        header={
          <span data-testid="prepare-progress-toggle" className={headerClass}>
            {headerLabel}
          </span>
        }
        hasExpandableContent={visibleSteps.length > 0 || hasSessionInfo}
        isExpanded={expanded}
        onToggle={() => setExpanded(!expanded)}
      >
        <div className="space-y-1">
          {visibleSteps.map((step, i) => (
            <StepRow key={i} step={step} />
          ))}
        </div>
        <SessionInfo sessionId={sessionId} />
      </ExpandableRow>
    </div>
  );
}
