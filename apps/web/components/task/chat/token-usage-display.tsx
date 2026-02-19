"use client";

import { memo } from "react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import { useSessionContextWindow } from "@/hooks/domains/session/use-session-context-window";

type TokenUsageDisplayProps = {
  sessionId: string | null;
  className?: string;
};

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

export const TokenUsageDisplay = memo(function TokenUsageDisplay({
  sessionId,
  className,
}: TokenUsageDisplayProps) {
  const contextWindow = useSessionContextWindow(sessionId);

  if (!contextWindow || contextWindow.size === 0) return null;

  const { size, used, efficiency } = contextWindow;

  const usagePercent = Math.min(efficiency, 100);
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
                className={cn(getCircleColor(efficiency), "transition-all duration-300 ease-out")}
              />
            </svg>
          </div>
        </div>
      </TooltipTrigger>
      <TooltipContent side="top">
        <div className="text-xs space-y-1">
          <div className="font-medium">
            {efficiency.toFixed(0)}% ({formatNumber(used)} / {formatNumber(size)})
          </div>
        </div>
      </TooltipContent>
    </Tooltip>
  );
});
