"use client";

import { IconAlertTriangle, IconLoader2 } from "@tabler/icons-react";

import type { PrepareSummary } from "@/lib/prepare/summarize";

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
