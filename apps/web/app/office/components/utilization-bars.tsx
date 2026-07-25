"use client";

import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { ProviderUsage } from "@/lib/state/slices/office/types";

type Props = {
  usage: ProviderUsage;
};

function getBarColor(pct: number): string {
  if (pct >= 90) return "bg-red-500";
  if (pct >= 80) return "bg-amber-500";
  return "bg-blue-500";
}

function getTextColor(pct: number): string {
  if (pct >= 90) return "text-red-600 dark:text-red-400";
  if (pct >= 80) return "text-amber-600 dark:text-amber-400";
  return "text-muted-foreground";
}

function formatResetTime(resetAt: string): string {
  const now = Date.now();
  const reset = new Date(resetAt).getTime();
  const diffMs = reset - now;
  if (diffMs <= 0) return "resetting soon";
  const diffH = Math.floor(diffMs / (1000 * 60 * 60));
  const diffM = Math.floor((diffMs % (1000 * 60 * 60)) / (1000 * 60));
  if (diffH > 0) return `Resets in ${diffH}h ${diffM}m`;
  return `Resets in ${diffM}m`;
}

function WindowBar({ label, pct, resetAt }: { label: string; pct: number; resetAt: string }) {
  const clampedPct = Math.min(100, Math.max(0, pct));
  const barColor = getBarColor(clampedPct);
  const textColor = getTextColor(clampedPct);

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div className="space-y-1 cursor-default">
          <div className="flex items-center justify-between text-xs">
            <span className="text-muted-foreground capitalize">{label}</span>
            <span className={`font-medium ${textColor}`}>{Math.round(clampedPct)}%</span>
          </div>
          <div className="h-2 bg-muted rounded-full overflow-hidden">
            <div
              className={`h-full rounded-full transition-all ${barColor}`}
              style={{ width: `${clampedPct}%` }}
            />
          </div>
        </div>
      </TooltipTrigger>
      <TooltipContent>
        <p className="text-xs">{formatResetTime(resetAt)}</p>
      </TooltipContent>
    </Tooltip>
  );
}

export function UtilizationBars({ usage }: Props) {
  if (!usage || usage.windows.length === 0) {
    return <p className="text-xs text-muted-foreground">No utilization data available.</p>;
  }

  return (
    <div className="space-y-3">
      {usage.windows.map((w) => (
        <WindowBar key={w.label} label={w.label} pct={w.utilization_pct} resetAt={w.reset_at} />
      ))}
    </div>
  );
}
