"use client";

import { memo } from "react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import { UsageWindowRows, usageStatus } from "@/components/usage/usage-window-rows";
import { useSessionContextWindow } from "@/hooks/domains/session/use-session-context-window";
import { useSessionAgentUsage } from "@/hooks/domains/session/use-session-agent-usage";

type TokenUsageDisplayProps = {
  sessionId: string | null;
  className?: string;
};

/**
 * A context-window report is only trustworthy when we have a positive window
 * size and usage that does not exceed it. `used > size` is impossible for a
 * real window, so it means the agent (via the ACP bridge) reported a stale or
 * wrong `size` (e.g. 200K for a model actually running on the 1M beta window).
 * In that case we hide the indicator instead of showing a confusing >100%.
 * `used === size` (exactly full) is valid and still renders.
 */
export function isContextWindowReliable(size: number, used: number): boolean {
  return size > 0 && used <= size;
}

function formatNumber(num: number): string {
  if (num >= 1_000_000) {
    return `${(num / 1_000_000).toFixed(1)}M`;
  }
  if (num >= 1_000) {
    return `${(num / 1_000).toFixed(1)}K`;
  }
  return num.toLocaleString();
}

function getCircleColor(efficiency: number): string {
  if (efficiency >= 90) return "text-yellow-500";
  if (efficiency >= 75) return "text-yellow-300";
  if (efficiency >= 50) return "text-blue-500";
  return "text-blue-300";
}

/**
 * Subscription utilization rows for the session's agent, rendered inside the
 * doughnut tooltip. Mounted only while the tooltip is open, so each hover
 * triggers a fresh provider fetch (server-clamped to one per 15 s).
 */
function SessionUsageRows({ sessionId }: { sessionId: string | null }) {
  const agentUsage = useSessionAgentUsage(sessionId);
  const usage = agentUsage?.usage;
  if (!usage || usage.windows.length === 0) return null;
  const status = usageStatus(usage);

  return (
    <div
      className="pt-2 mt-2 border-t border-border/60 space-y-2 min-w-56"
      data-testid="doughnut-subscription-usage"
    >
      <div className="flex items-center justify-between gap-4">
        <span className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
          Subscription usage{usage.plan ? ` · ${usage.plan}` : ""}
        </span>
        <span className={cn("text-[10px] font-semibold", status.className)}>{status.label}</span>
      </div>
      <UsageWindowRows usage={usage} />
    </div>
  );
}

export const TokenUsageDisplay = memo(function TokenUsageDisplay({
  sessionId,
  className,
}: TokenUsageDisplayProps) {
  const contextWindow = useSessionContextWindow(sessionId);

  if (!contextWindow) return null;

  const { size, used } = contextWindow;

  // Hide when there's no data yet (size 0) or the report is impossible
  // (used > size) — see isContextWindowReliable.
  if (!isContextWindowReliable(size, used)) return null;

  const usagePercent = (used / size) * 100;
  const progress = usagePercent / 100;

  // SVG circle parameters
  const radius = 10;
  const strokeWidth = 2.5;
  const circumference = 2 * Math.PI * radius;
  const strokeDashoffset = circumference * (1 - progress);

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div className={cn("flex items-center gap-2 cursor-help", className)}>
          <div className="relative flex items-center justify-center">
            <svg viewBox="0 0 24 24" className="w-5 h-5 -rotate-90" aria-hidden="true">
              {/* Background circle */}
              <circle
                cx="12"
                cy="12"
                r={radius}
                fill="none"
                stroke="currentColor"
                strokeWidth={strokeWidth}
                className="text-muted"
              />
              {/* Progress circle */}
              <circle
                cx="12"
                cy="12"
                r={radius}
                fill="none"
                stroke="currentColor"
                strokeWidth={strokeWidth}
                strokeLinecap="round"
                strokeDasharray={circumference}
                strokeDashoffset={strokeDashoffset}
                className={cn(getCircleColor(usagePercent), "transition-all duration-300 ease-out")}
              />
            </svg>
          </div>
        </div>
      </TooltipTrigger>
      <TooltipContent side="top">
        <div className="text-xs space-y-1">
          <div className="font-medium">
            {usagePercent.toFixed(0)}% ({formatNumber(used)} / {formatNumber(size)})
          </div>
          <SessionUsageRows sessionId={sessionId} />
        </div>
      </TooltipContent>
    </Tooltip>
  );
});
