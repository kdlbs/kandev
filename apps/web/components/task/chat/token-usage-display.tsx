'use client';

import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { cn } from '@/lib/utils';
import type { ContextWindowEntry } from '@/lib/state/store';

type TokenUsageDisplayProps = {
  contextWindow: ContextWindowEntry;
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

function getBarColor(efficiency: number): string {
  if (efficiency >= 90) return 'bg-red-500';
  if (efficiency >= 75) return 'bg-orange-500';
  if (efficiency >= 50) return 'bg-yellow-500';
  return 'bg-emerald-500';
}

export function TokenUsageDisplay({ contextWindow, className }: TokenUsageDisplayProps) {
  const { size, used, remaining, efficiency } = contextWindow;

  if (size === 0) return null;

  const usagePercent = Math.min(efficiency, 100);

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div className={cn('flex items-center gap-1.5 cursor-default', className)}>
          <div className="w-16 h-1.5 rounded-full bg-muted/50 overflow-hidden">
            <div
              className={cn('h-full rounded-full transition-all', getBarColor(efficiency))}
              style={{ width: `${usagePercent}%` }}
            />
          </div>
          <span className="text-[10px] text-muted-foreground tabular-nums">
            {efficiency.toFixed(0)}%
          </span>
        </div>
      </TooltipTrigger>
      <TooltipContent side="top">
        <div className="text-xs space-y-1">
          <div className="font-medium">Context: {formatNumber(used)} / {formatNumber(size)}</div>
          <div className="text-muted-foreground">{formatNumber(remaining)} remaining</div>
        </div>
      </TooltipContent>
    </Tooltip>
  );
}

