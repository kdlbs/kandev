"use client";

import { useTranslation } from "react-i18next";
import { IconChevronRight } from "@tabler/icons-react";
import Link from "@/components/routing/app-link";
import { formatRelativeTime } from "@/lib/utils";
import type { Automation, AutomationRun } from "@/lib/types/automation";
import { cn } from "@/lib/utils";
import { describeAutomationSchedule, scheduleBinding } from "./automation-schedule";
import { RunDetailDisclosure } from "./run-detail-disclosure";
import { groupRunsByState, statusDotClass, statusLabelKey } from "./run-status";
import { detailTabHref } from "./runs-view";

/**
 * The rail is a switcher, not a log.
 *
 * A run is one instance of a conversation that recurs, so the thing the reader
 * does here is move between instances — not read them. Each row is therefore a
 * time and a state and nothing else; what the run actually said is in the pane
 * beside it, at full size.
 */
type RunsRailProps = {
  automation: Automation;
  runs: AutomationRun[];
  selectedRunId: string | null;
  onSelect: (run: AutomationRun) => void;
  /** Feeds the run-detail panel's next-firing line. */
  openRuns: number;
  width: number;
  resizing: boolean;
  onResizeStart: (event: React.MouseEvent) => void;
};

function RunRow({
  run,
  selected,
  onSelect,
}: {
  run: AutomationRun;
  selected: boolean;
  onSelect: (run: AutomationRun) => void;
}) {
  const { t } = useTranslation();
  // A run with no conversation has nothing to switch to. It still reports
  // itself — a skipped firing is the whole story — but it does not pretend to
  // be openable.
  const openable = Boolean(run.session_id);
  const body = (
    <>
      <span
        className={cn("mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full", statusDotClass(run.status))}
        aria-hidden="true"
      />
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="truncate text-[13px] text-foreground">
          {run.display_title || formatRelativeTime(run.created_at)}
        </span>
        {run.display_title && (
          <span className="truncate text-[11px] text-muted-foreground">
            {formatRelativeTime(run.created_at)}
          </span>
        )}
        <span className="truncate text-xs text-muted-foreground">
          {t(statusLabelKey(run.status))}
        </span>
      </span>
      {openable && (
        <IconChevronRight className="mt-1 h-3.5 w-3.5 shrink-0 text-muted-foreground/50" />
      )}
    </>
  );

  const shared = "flex w-full gap-2.5 rounded-md px-2.5 py-2 text-left";
  if (!openable) {
    return (
      <div className={cn(shared, "opacity-70")} data-testid={`run-row-${run.id}`}>
        {body}
      </div>
    );
  }
  return (
    <button
      type="button"
      onClick={() => onSelect(run)}
      aria-current={selected ? "true" : undefined}
      data-testid={`run-row-${run.id}`}
      className={cn(
        shared,
        "cursor-pointer transition-colors",
        selected ? "bg-muted/70" : "hover:bg-muted/40",
      )}
    >
      {body}
    </button>
  );
}

export function RunGroup({
  title,
  groupId,
  runs,
  selectedRunId,
  onSelect,
}: {
  title: string;
  /** Stable identity for tooling — the title is translated, this is not. */
  groupId: string;
  runs: AutomationRun[];
  selectedRunId: string | null;
  onSelect: (run: AutomationRun) => void;
}) {
  if (runs.length === 0) return null;
  return (
    <div className="flex flex-col gap-0.5" data-testid={`run-group-${groupId}`}>
      <p className="px-2.5 pt-3 pb-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/70">
        {title}
      </p>
      {runs.map((run) => (
        <RunRow key={run.id} run={run} selected={run.id === selectedRunId} onSelect={onSelect} />
      ))}
    </div>
  );
}

export function RunsRail({
  automation,
  runs,
  selectedRunId,
  onSelect,
  openRuns,
  width,
  resizing,
  onResizeStart,
}: RunsRailProps) {
  const { t } = useTranslation();
  const { running, completed } = groupRunsByState(runs);
  const hasSchedule = Boolean(scheduleBinding(automation).expression);

  return (
    <aside
      className={cn(
        "relative flex shrink-0 flex-col border-l border-border/60 bg-background",
        // No width transition while dragging, or the edge lags the cursor.
        !resizing && "transition-[width] duration-150",
      )}
      style={{ width }}
      data-testid="runs-rail"
    >
      {/* Same drag affordance as the sidebar's edge, so both behave alike. */}
      <button
        type="button"
        aria-label={t("automations:resizeRuns")}
        onMouseDown={onResizeStart}
        tabIndex={-1}
        data-testid="runs-rail-resize"
        className="absolute -left-px top-0 z-10 h-full w-1 -translate-x-1/2 cursor-ew-resize bg-transparent transition-colors hover:bg-primary active:bg-primary"
      />
      <div className="flex items-baseline justify-between gap-2 border-b border-border/60 px-3 py-3">
        <div className="min-w-0">
          <p className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/70">
            {t("automations:runs")}
          </p>
          {/* The schedule is a property of the automation, not a settings
              screen — stating it here is what makes the times below legible. */}
          {hasSchedule && (
            <p className="mt-0.5 truncate text-xs text-muted-foreground">
              {describeAutomationSchedule(automation)}
            </p>
          )}
        </div>
        <Link
          href={detailTabHref(automation.id, "configure")}
          className="shrink-0 cursor-pointer text-xs text-muted-foreground hover:text-foreground transition-colors"
          data-testid="automation-details-link"
        >
          {t("automations:details")}
        </Link>
      </div>
      {/* The run view's old header block, now asked for rather than imposed. */}
      <RunDetailDisclosure
        automation={automation}
        openRuns={openRuns}
        className="shrink-0 border-b border-border/60 px-1.5 py-1"
      />
      <div className="min-h-0 flex-1 overflow-y-auto px-1.5 pb-3">
        {runs.length === 0 ? (
          <p className="px-2.5 py-6 text-xs text-muted-foreground" data-testid="runs-rail-empty">
            {t("automations:noRunsYet")}
          </p>
        ) : (
          <>
            <RunGroup
              title={t("automations:running")}
              groupId="running"
              runs={running}
              selectedRunId={selectedRunId}
              onSelect={onSelect}
            />
            <RunGroup
              title={t("automations:completed")}
              groupId="completed"
              runs={completed}
              selectedRunId={selectedRunId}
              onSelect={onSelect}
            />
          </>
        )}
      </div>
    </aside>
  );
}
