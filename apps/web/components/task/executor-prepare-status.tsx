"use client";

import { useMemo } from "react";
import { IconAlertTriangle, IconLoader2 } from "@tabler/icons-react";

import { useAppStore } from "@/components/state-provider";
import type {
  PrepareStepInfo,
  SessionPrepareState,
} from "@/lib/state/slices/session-runtime/types";

export type PreparePhase =
  | "idle"
  | "preparing"
  | "preparing_fallback"
  | "resuming"
  | "ready"
  | "failed";

export type PrepareSummary = {
  phase: PreparePhase;
  current: PrepareStepInfo | null;
  currentIndex: number;
  totalSteps: number;
  fallbackWarning: string | null;
  failedStep: PrepareStepInfo | null;
  durationMs: number | null;
};

const IDLE_PREPARE_SUMMARY: PrepareSummary = {
  phase: "idle",
  current: null,
  currentIndex: 0,
  totalSteps: 0,
  fallbackWarning: null,
  failedStep: null,
  durationMs: null,
};

/**
 * Reduces the prepare-progress slice + session state for one session into a
 * UI-friendly summary. The reducer is the only place that decides what counts
 * as "still preparing" vs "ready" vs "failed", and it surfaces:
 *  - the missing-sandbox fallback as a first-class "preparing_fallback" phase
 *    so the popover renders dedicated UI for the recovery path;
 *  - a "resuming" phase synthesized from session.state == STARTING when the
 *    backend skips prepare events on resume, so the popover still shows a
 *    spinner instead of looking idle for the entire reconnect.
 */
export function usePrepareSummary(sessionId: string | null): PrepareSummary {
  const prepareState = useAppStore((state) =>
    sessionId ? (state.prepareProgress?.bySessionId?.[sessionId] ?? null) : null,
  );
  const sessionState = useAppStore((state) =>
    sessionId ? (state.taskSessions?.items?.[sessionId]?.state ?? null) : null,
  );
  return useMemo(() => summarizePrepare(prepareState, sessionState), [prepareState, sessionState]);
}

export function isPreparingPhase(phase: PreparePhase): boolean {
  return phase === "preparing" || phase === "preparing_fallback" || phase === "resuming";
}

function summarizePrepare(
  state: SessionPrepareState | null,
  sessionState: string | null,
): PrepareSummary {
  if (!state) {
    if (sessionState === "STARTING") {
      return { ...IDLE_PREPARE_SUMMARY, phase: "resuming" };
    }
    return IDLE_PREPARE_SUMMARY;
  }

  const steps = state.steps ?? [];
  const fallbackStep = steps.find(isFallbackNoticeStep) ?? null;
  const failedStep = steps.find((s) => s.status === "failed") ?? null;
  const runningStep = steps.find((s) => s.status === "running") ?? null;
  const { current, currentIndex } = pickCurrentStep(steps, runningStep);

  return {
    phase: derivePhase(state.status, fallbackStep, failedStep),
    current,
    currentIndex,
    totalSteps: steps.length,
    fallbackWarning: fallbackStep?.warning ?? null,
    failedStep,
    durationMs: state.durationMs ?? null,
  };
}

function pickCurrentStep(
  steps: PrepareStepInfo[],
  runningStep: PrepareStepInfo | null,
): { current: PrepareStepInfo | null; currentIndex: number } {
  if (runningStep) {
    return { current: runningStep, currentIndex: steps.indexOf(runningStep) };
  }
  const lastCompletedIdx = lastIndexWhere(
    steps,
    (s) => s.status === "completed" || s.status === "skipped" || s.status === "failed",
  );
  if (lastCompletedIdx >= 0) {
    return { current: steps[lastCompletedIdx], currentIndex: lastCompletedIdx };
  }
  return { current: steps[0] ?? null, currentIndex: 0 };
}

function derivePhase(
  status: string,
  fallbackStep: PrepareStepInfo | null,
  failedStep: PrepareStepInfo | null,
): PreparePhase {
  if (status === "preparing") return fallbackStep ? "preparing_fallback" : "preparing";
  if (status === "failed" || failedStep) return "failed";
  if (status === "completed") return "ready";
  return "idle";
}

function isFallbackNoticeStep(step: PrepareStepInfo): boolean {
  return step.status === "skipped" && step.name === "Reconnecting cloud sandbox";
}

function lastIndexWhere<T>(items: T[], pred: (item: T) => boolean): number {
  for (let i = items.length - 1; i >= 0; i--) {
    if (pred(items[i])) return i;
  }
  return -1;
}

export function PrepareStatusSection({ summary }: { summary: PrepareSummary }) {
  if (summary.phase === "idle" || summary.phase === "ready") return null;
  if (summary.phase === "resuming") return <ResumingRow />;
  if (summary.phase === "preparing" || summary.phase === "preparing_fallback") {
    return <PreparingRow summary={summary} />;
  }
  if (summary.phase === "failed") return <FailedRow summary={summary} />;
  return null;
}

function ResumingRow() {
  return (
    <div
      data-testid="executor-prepare-status"
      data-phase="resuming"
      className="border-b border-border px-3 py-2.5 space-y-1"
    >
      <div className="flex items-center gap-2">
        <IconLoader2 className="h-3.5 w-3.5 animate-spin text-muted-foreground" />
        <span className="font-medium text-foreground">Resuming session</span>
      </div>
      <div className="text-xs text-muted-foreground">Reconnecting to the existing environment…</div>
    </div>
  );
}

function PreparingRow({ summary }: { summary: PrepareSummary }) {
  const stepLabel = summary.current?.name ?? "Preparing...";
  const stepNumber = Math.min(summary.currentIndex + 1, summary.totalSteps);
  return (
    <div
      data-testid="executor-prepare-status"
      data-phase={summary.phase}
      className="border-b border-border px-3 py-2.5 space-y-1"
    >
      <div className="flex items-center gap-2">
        <IconLoader2 className="h-3.5 w-3.5 animate-spin text-muted-foreground" />
        <span className="font-medium text-foreground">Preparing environment</span>
      </div>
      <div className="text-xs text-muted-foreground">
        {summary.totalSteps > 0
          ? `Step ${stepNumber} of ${summary.totalSteps}: ${stepLabel}`
          : stepLabel}
      </div>
      {summary.phase === "preparing_fallback" && summary.fallbackWarning && (
        <div
          data-testid="executor-prepare-fallback-warning"
          className="mt-1 flex items-start gap-1.5 text-xs text-amber-600 dark:text-amber-400"
        >
          <IconAlertTriangle className="h-3.5 w-3.5 flex-shrink-0 mt-0.5" />
          <span>{summary.fallbackWarning}</span>
        </div>
      )}
    </div>
  );
}

function FailedRow({ summary }: { summary: PrepareSummary }) {
  return (
    <div
      data-testid="executor-prepare-status"
      data-phase="failed"
      className="border-b border-border px-3 py-2.5 space-y-1"
    >
      <div className="flex items-center gap-2 text-destructive">
        <IconAlertTriangle className="h-3.5 w-3.5" />
        <span className="font-medium">Environment preparation failed</span>
      </div>
      {summary.failedStep && (
        <div className="text-xs text-muted-foreground">Failed at: {summary.failedStep.name}</div>
      )}
      {summary.failedStep?.error && (
        <pre className="text-[11px] text-destructive whitespace-pre-wrap max-h-16 overflow-auto">
          {summary.failedStep.error}
        </pre>
      )}
    </div>
  );
}
