"use client";

import { IconPointFilled } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";

type ExecutionIndicatorProps = {
  status: string;
  className?: string;
};

/**
 * Shows a live/ready indicator for tasks with an active agent execution.
 * - IN_PROGRESS / SCHEDULING: pulsing green dot + "Live"
 * - REVIEW / WAITING: solid amber dot + "Ready"
 * - Otherwise: hidden
 */
export function ExecutionIndicator({ status, className }: ExecutionIndicatorProps) {
  const normalized = status?.toLowerCase().replace(/ /g, "_");

  if (normalized === "in_progress" || normalized === "scheduling") {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <span
            className={cn("inline-flex items-center gap-1 text-xs text-emerald-500", className)}
          >
            <IconPointFilled className="h-3 w-3 animate-pulse" />
            Live
          </span>
        </TooltipTrigger>
        <TooltipContent>Agent is actively working on this task</TooltipContent>
      </Tooltip>
    );
  }

  if (normalized === "in_review" || normalized === "review") {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <span className={cn("inline-flex items-center gap-1 text-xs text-amber-500", className)}>
            <IconPointFilled className="h-3 w-3" />
            Ready
          </span>
        </TooltipTrigger>
        <TooltipContent>Agent finished — workspace ready for review</TooltipContent>
      </Tooltip>
    );
  }

  return null;
}
