"use client";

import type { ReactNode } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { linkToTask } from "@/lib/links";
import type { OverviewTaskFilter, OverviewWorkspaceMetrics } from "@/lib/types/control-overview";
import { occurredTime, relativeTime } from "./overview-format";

/**
 * One metric of the project card. It sits on its own tile so the card reads as
 * a set of figures rather than a wall of numbers; the tile the filter has
 * selected is lifted off the tile color so the current filter is visible
 * without reading the pressed state of the chip.
 */
export function Metric({
  label,
  value,
  sub,
  title,
  onClick,
  href,
  active,
}: {
  label: string;
  value: ReactNode;
  sub?: ReactNode;
  /**
   * The reading behind the value, for hover. A metric shows the fixed instant a
   * thing happened and carries the relative reading here, so an operator can ask
   * "how long ago?" without the number on the tile having to restate itself.
   */
  title?: string;
  onClick?: () => void;
  href?: string;
  active?: boolean;
}) {
  const surface = active
    ? "border-border bg-muted"
    : "border-tile-border bg-tile hover:bg-muted/60";
  const body = (
    <>
      <div className="text-[11px] text-muted-foreground">{label}</div>
      <div className="text-lg font-semibold tabular-nums" title={title}>
        {value}
      </div>
      <div className="min-h-3 text-[11px] text-muted-foreground">{sub}</div>
    </>
  );
  const className = `block rounded-md border p-2 text-center transition-colors cursor-pointer ${surface}`;
  if (href) {
    return (
      <Link href={href} className={className}>
        {body}
      </Link>
    );
  }
  if (onClick) {
    return (
      <button type="button" className={className} onClick={onClick}>
        {body}
      </button>
    );
  }
  return <div className={`rounded-md border p-2 text-center ${surface}`}>{body}</div>;
}

/**
 * The last-output tile's hover reading: how long ago the output happened, and
 * what the tile opens. The tile is the one figure that is a link rather than a
 * filter, so the hint names the task the link goes to: the task whose most
 * recent live session produced that output.
 */
function lastOutputTitle(t: TFunction, at: string | undefined): string {
  const hint = t("office:overviewLastOutputHint");
  return at ? `${relativeTime(at)} · ${hint}` : hint;
}

/** The project's six figures, each one a tile and each one a filter. */
export function WorkspaceMetrics({
  metrics,
  filter,
  onFilter,
}: {
  metrics: OverviewWorkspaceMetrics;
  filter: OverviewTaskFilter | null;
  onFilter: (filter: OverviewTaskFilter) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="grid grid-cols-2 gap-1 px-2 pt-2 sm:grid-cols-3 lg:grid-cols-6">
      <Metric
        label={t("office:overviewActiveTasks")}
        value={metrics.active_tasks}
        onClick={() => onFilter("active")}
        active={filter === "active"}
      />
      <Metric
        label={t("sidebar:controlRestore.processRunning")}
        value={metrics.running_sessions}
        sub={t("office:overviewWaitingInput", { count: metrics.waiting_input_sessions })}
        onClick={() => onFilter("sessions")}
        active={filter === "sessions"}
      />
      <Metric
        label={t("office:overviewLastOutput")}
        value={metrics.last_output_at ? occurredTime(metrics.last_output_at) : "-"}
        title={metrics.last_output_task_id ? lastOutputTitle(t, metrics.last_output_at) : undefined}
        href={metrics.last_output_task_id ? linkToTask(metrics.last_output_task_id) : undefined}
      />
      <Metric
        label={t("chat:queuedMessages")}
        value={metrics.queued_messages}
        onClick={() => onFilter("queued")}
        active={filter === "queued"}
      />
      <Metric
        label={t("office:overviewCompleted24h")}
        value={metrics.completed_24h}
        onClick={() => onFilter("completed")}
        active={filter === "completed"}
      />
      <Metric
        label={t("office:overviewOpenTasks")}
        value={metrics.open_tasks}
        // The three figures partition the open tasks: a task is on hold, or
        // waiting on a predecessor, or neither. "Other" carries the tasks in
        // progress along with the ones waiting on a person, so it is not worded
        // as if none of them were moving.
        sub={t("office:overviewOpenBreakdown", {
          hold: metrics.blocked_tasks,
          blocked: metrics.blocked_by_tasks,
          other: metrics.open_tasks - metrics.blocked_tasks - metrics.blocked_by_tasks,
        })}
        onClick={() => onFilter("all")}
        active={filter === "all"}
      />
    </div>
  );
}
