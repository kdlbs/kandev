"use client";

import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { formatRelativeTime } from "@/lib/utils";
import type { AutomationRun } from "@/lib/types/automation";
import { isOpenRun, statusDotClass, statusLabelKey } from "./run-status";
import { t } from "@/lib/i18n";

/**
 * What the run actually said, in priority order. An error outranks the summary
 * — a failed run's summary is usually the last thing the agent managed to say
 * before it went wrong, which reads as success if shown on its own.
 */
export function outcomeText(run: AutomationRun): string {
  if (run.error_message) return run.error_message;
  if (run.summary) return run.summary;
  if (isOpenRun(run.status)) return t("automations:runStillRunningNoReport");
  return t("automations:runNoReportRecorded");
}

type RunFeedItemProps = {
  run: AutomationRun;
  /**
   * Only the cross-automation lens needs this. Inside one automation's own
   * history, repeating its name on every entry is noise the reader already
   * knows — the status takes the line instead.
   */
  automationName?: string;
  onOpen: (taskId: string) => void;
};

function RunFeedItemBody({ run, automationName }: { run: AutomationRun; automationName?: string }) {
  const { t } = useTranslation();
  const outcome = outcomeText(run);
  // A skipped firing stores its reason in error_message, but a skip is the
  // concurrency cap working, not a failure. Rendering it red made a correctly
  // throttled automation look broken at a glance.
  const isError = Boolean(run.error_message) && run.status !== "skipped";
  return (
    <>
      <span
        className={cn("mt-1.5 h-2 w-2 shrink-0 rounded-full", statusDotClass(run.status))}
        aria-hidden="true"
      />
      <span className="flex min-w-0 flex-1 flex-col gap-1">
        <span className="flex items-baseline justify-between gap-3">
          <span className="truncate text-sm font-medium text-foreground">
            {automationName ?? t(statusLabelKey(run.status))}
          </span>
          <span className="shrink-0 text-xs text-muted-foreground whitespace-nowrap">
            {formatRelativeTime(run.created_at)}
          </span>
        </span>
        {/* The report is the deliverable, so it gets two full lines of the
            entry — clamped rather than truncated to one so a finding survives
            the summary line it usually starts on. */}
        <span
          className={cn(
            "line-clamp-2 text-sm leading-relaxed",
            isError ? "text-destructive" : "text-muted-foreground",
          )}
          title={outcome}
          data-testid="run-outcome"
        >
          {outcome}
        </span>
        {automationName && (
          <span className="text-xs text-muted-foreground/80" data-testid="run-status-label">
            {t(statusLabelKey(run.status))}
          </span>
        )}
      </span>
    </>
  );
}

export function RunFeedItem({ run, automationName, onOpen }: RunFeedItemProps) {
  const baseClass =
    "flex w-full gap-3 rounded-md border border-transparent px-3 py-3 text-left transition-colors";
  const testId = `run-entry-${run.id}`;

  // A run that never produced a task has no transcript to open — a skipped
  // schedule is the whole story already — so it renders inert rather than
  // offering a click that would go nowhere.
  if (!run.task_id) {
    return (
      <div className={cn(baseClass, "opacity-80")} data-testid={testId}>
        <RunFeedItemBody run={run} automationName={automationName} />
      </div>
    );
  }

  return (
    <button
      type="button"
      className={cn(baseClass, "cursor-pointer hover:border-border hover:bg-muted/40")}
      onClick={() => onOpen(run.task_id)}
      data-testid={testId}
    >
      <RunFeedItemBody run={run} automationName={automationName} />
    </button>
  );
}
