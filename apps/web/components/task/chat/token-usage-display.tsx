"use client";

import { memo, useEffect, useId, useRef, useState } from "react";
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

function usePinnableTooltip() {
  const [open, setOpen] = useState(false);
  const pinnedRef = useRef(false);
  const triggerRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    const closePinnedTooltip = () => {
      pinnedRef.current = false;
      setOpen(false);
    };
    const closeOnOutsidePointer = (event: PointerEvent) => {
      if (!pinnedRef.current || !(event.target instanceof Node)) return;
      if (
        triggerRef.current?.contains(event.target) ||
        (event.target instanceof Element && event.target.closest('[data-slot="tooltip-content"]'))
      ) {
        return;
      }
      closePinnedTooltip();
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape" && pinnedRef.current) closePinnedTooltip();
    };

    document.addEventListener("pointerdown", closeOnOutsidePointer);
    document.addEventListener("keydown", closeOnEscape);
    return () => {
      document.removeEventListener("pointerdown", closeOnOutsidePointer);
      document.removeEventListener("keydown", closeOnEscape);
    };
  }, []);

  return {
    open,
    triggerRef,
    onOpenChange: (nextOpen: boolean) => {
      if (!nextOpen && pinnedRef.current) return;
      setOpen(nextOpen);
    },
    onTriggerClick: () => {
      pinnedRef.current = !pinnedRef.current;
      setOpen(pinnedRef.current);
    },
  };
}

function ContextWindowRing({ usagePercent }: { usagePercent: number }) {
  const radius = 10;
  const strokeWidth = 2.5;
  const circumference = 2 * Math.PI * radius;
  const strokeDashoffset = circumference * (1 - usagePercent / 100);

  return (
    <svg viewBox="0 0 24 24" className="size-5 -rotate-90" aria-hidden="true">
      <circle
        cx="12"
        cy="12"
        r={radius}
        fill="none"
        stroke="currentColor"
        strokeWidth={strokeWidth}
        className="text-muted"
      />
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
  );
}

function ContextWindowSource({ source }: { source: "acp" | "api" | undefined }) {
  const helpId = useId();

  if (!source) return null;

  return (
    <div className="group relative flex shrink-0 items-center gap-1 text-[10px] text-muted-foreground">
      <span>Source</span>
      <span className="font-medium text-foreground">{source.toUpperCase()}</span>
      <button
        type="button"
        aria-label="About context window source"
        aria-describedby={helpId}
        className="inline-flex size-6 cursor-help items-center justify-center text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring sm:size-4"
      >
        <IconInfoCircle className="h-3 w-3" />
      </button>
      <span
        id={helpId}
        role="tooltip"
        className="pointer-events-none absolute right-0 bottom-[calc(100%+0.375rem)] z-10 w-60 rounded-md border border-border bg-popover px-3 py-1.5 text-xs text-popover-foreground opacity-0 shadow-sm transition-opacity group-hover:opacity-100 group-focus-within:opacity-100"
      >
        ACP is the active session&apos;s effective window, reported by the agent. API is the
        model&apos;s advertised maximum from the catalogue and is used when ACP omits the window.
      </span>
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
  const tooltip = usePinnableTooltip();
  const contextWindow = useSessionContextWindow(sessionId);

  if (!contextWindow) return null;

  const { size, used, source } = contextWindow;

  // Hide when there's no data yet (size 0) or the report is impossible
  // (used > size) — see isContextWindowReliable.
  if (!isContextWindowReliable(size, used)) return null;

  const usagePercent = (used / size) * 100;

  return (
    // The UI wrapper defaults this to true; the source control must remain reachable inside.
    <TooltipProvider disableHoverableContent={false}>
      <Tooltip open={tooltip.open} onOpenChange={tooltip.onOpenChange}>
        <TooltipTrigger asChild>
          <button
            ref={tooltip.triggerRef}
            type="button"
            aria-label={`Context window: ${usagePercent.toFixed(0)}% used`}
            aria-expanded={tooltip.open}
            onClick={tooltip.onTriggerClick}
            className={cn(
              "flex size-7 cursor-help items-center justify-center rounded-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring sm:size-5",
              className,
            )}
          >
            <ContextWindowRing usagePercent={usagePercent} />
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
              <div
                className="flex min-h-6 items-center justify-between gap-3"
                data-testid="context-window-token-row"
              >
                <span className="text-[11px] tabular-nums text-muted-foreground">
                  {formatNumber(used)} of {formatNumber(size)} tokens
                </span>
                <ContextWindowSource source={source} />
              </div>
            </div>
            <SessionUsageRows sessionId={sessionId} />
          </div>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
});
