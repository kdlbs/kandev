"use client";

import { Fragment } from "react";
import { cn } from "@/lib/utils";
import type { ProviderUsage, UtilizationWindow } from "@/lib/types/agent-profile";
import { getBarColor, getTextColor } from "./utilization-bars";

/** "5-hour" → "5h", "7-day (Opus)" → "7d (Opus)". */
export function shortWindowLabel(label: string): string {
  return label.replace(/(\d+)-hour/i, "$1h").replace(/(\d+)-day/i, "$1d");
}

/** Compact time-until-reset: "3h 3m", "5d 12h", "42m", "soon". */
export function formatResetShort(resetAt: string, now = Date.now()): string {
  const diffMs = new Date(resetAt).getTime() - now;
  if (!Number.isFinite(diffMs) || diffMs <= 0) return "soon";
  const totalHours = Math.floor(diffMs / 3_600_000);
  const minutes = Math.floor((diffMs % 3_600_000) / 60_000);
  if (totalHours >= 24) return `${Math.floor(totalHours / 24)}d ${totalHours % 24}h`;
  if (totalHours > 0) return `${totalHours}h ${minutes}m`;
  return `${minutes}m`;
}

export type UsageStatus = {
  label: "Good" | "High" | "Critical";
  className: string;
};

/** Worst-window health for a provider, rendered like "Good" / "Critical". */
export function usageStatus(usage: ProviderUsage): UsageStatus {
  const worst = Math.max(0, ...usage.windows.map((w) => w.utilization_pct));
  if (worst >= 90) return { label: "Critical", className: "text-red-600 dark:text-red-400" };
  if (worst >= 80) return { label: "High", className: "text-amber-600 dark:text-amber-400" };
  return { label: "Good", className: "text-emerald-600 dark:text-emerald-400" };
}

function UsageWindowCells({ window: w }: { window: UtilizationWindow }) {
  const pct = Math.min(100, Math.max(0, w.utilization_pct));
  return (
    <>
      <span className="text-muted-foreground whitespace-nowrap">{shortWindowLabel(w.label)}</span>
      <div className="h-1.5 min-w-20 rounded-full bg-muted overflow-hidden">
        <div
          className={cn("h-full rounded-full transition-all", getBarColor(pct))}
          style={{ width: `${pct}%` }}
        />
      </div>
      <span className={cn("font-semibold tabular-nums text-right", getTextColor(pct))}>
        {Math.round(pct)}%
      </span>
      <span
        className="text-muted-foreground/70 tabular-nums text-right whitespace-nowrap"
        title={`Resets in ${formatResetShort(w.reset_at)}`}
      >
        {formatResetShort(w.reset_at)}
      </span>
    </>
  );
}

/**
 * Compact per-window utilization rows: `5h [====   ] 39%  3h 3m`.
 * Shared by the settings usage cards and the chat doughnut tooltip.
 */
export function UsageWindowRows({
  usage,
  className,
}: {
  usage: ProviderUsage;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "grid grid-cols-[auto_1fr_auto_auto] items-center gap-x-3 gap-y-2 text-xs",
        className,
      )}
    >
      {usage.windows.map((w) => (
        <Fragment key={w.label}>
          <UsageWindowCells window={w} />
        </Fragment>
      ))}
    </div>
  );
}
