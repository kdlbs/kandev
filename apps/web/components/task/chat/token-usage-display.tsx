"use client";

import { memo, useState } from "react";
import { IconInfoCircle } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
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
 * wrong `size` (for example, usage and window metadata from different turns).
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

function ContextWindowSource({ source }: { source: "acp" | "api" | undefined }) {
  if (!source) return null;

  return (
    <div className="flex items-center gap-1 text-[10px] text-muted-foreground">
      <span>Source</span>
      <span className="font-medium text-foreground">{source.toUpperCase()}</span>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            aria-label="About context window source"
            className="inline-flex cursor-help text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
          >
            <IconInfoCircle className="h-3 w-3" />
          </button>
        </TooltipTrigger>
        <TooltipContent side="top" className="max-w-60">
          ACP is the active session&apos;s effective window, reported by the agent. API is the
          model&apos;s advertised maximum from the catalogue and is used when ACP omits the window.
        </TooltipContent>
      </Tooltip>
    </div>
  );
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
      className="pt-2 mt-2 border-t border-border/60 space-y-2 min-w-64 opacity-80"
      data-testid="doughnut-subscription-usage"
    >
      <div className="flex items-center justify-between gap-4">
        <span className="text-[10px] font-medium uppercase text-muted-foreground">
          Subscription{usage.plan ? ` · ${usage.plan}` : ""}
        </span>
        <span className={cn("text-[10px] font-medium", status.className)}>{status.label}</span>
      </div>
      <UsageWindowRows usage={usage} className="text-[11px]" />
    </div>
  );
}

export const TokenUsageDisplay = memo(function TokenUsageDisplay({
  sessionId,
  className,
}: TokenUsageDisplayProps) {
  const [tooltipOpen, setTooltipOpen] = useState(false);
  const contextWindow = useSessionContextWindow(sessionId);

  if (!contextWindow) return null;

  const { size, used, source } = contextWindow;

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
    // The UI wrapper defaults this to true; nested source help must remain reachable.
    <TooltipProvider disableHoverableContent={false}>
      <Tooltip open={tooltipOpen} onOpenChange={setTooltipOpen}>
        <TooltipTrigger asChild>
          <button
            type="button"
            aria-label={`Context window: ${usagePercent.toFixed(0)}% used`}
            onClick={() => setTooltipOpen(true)}
            className={cn(
              "flex size-7 cursor-help items-center justify-center rounded-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring sm:size-5",
              className,
            )}
          >
            <svg viewBox="0 0 24 24" className="size-5 -rotate-90" aria-hidden="true">
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
          </button>
        </TooltipTrigger>
        <TooltipContent side="top" className="pointer-events-auto">
          <div className="min-w-64 text-xs">
            <div className="space-y-2" data-testid="context-window-usage">
              <div className="flex items-baseline justify-between gap-6">
                <span className="text-[10px] font-medium uppercase text-muted-foreground">
                  Context window
                </span>
                <span className="text-base font-semibold tabular-nums text-foreground">
                  {usagePercent.toFixed(0)}%
                </span>
              </div>
              <div
                className={cn(
                  "h-1.5 overflow-hidden rounded-full bg-muted",
                  getCircleColor(usagePercent),
                )}
              >
                <div
                  className="h-full rounded-full bg-current transition-all duration-300 ease-out"
                  style={{ width: `${usagePercent}%` }}
                />
              </div>
              <div className="text-[11px] tabular-nums text-muted-foreground">
                {formatNumber(used)} of {formatNumber(size)} tokens
              </div>
              <ContextWindowSource source={source} />
            </div>
            <SessionUsageRows sessionId={sessionId} />
          </div>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
});
