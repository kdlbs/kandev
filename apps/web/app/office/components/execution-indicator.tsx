"use client";

import { IconPointFilled } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { CompositorPulse } from "@kandev/ui/compositor-pulse";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";

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
  const { t } = useTranslation();
  const normalized = status?.toLowerCase().replace(/ /g, "_");

  if (normalized === "in_progress" || normalized === "scheduling") {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <span
            className={cn("inline-flex items-center gap-1 text-xs text-emerald-500", className)}
          >
            <CompositorPulse className="inline-flex animate-pulse">
              <IconPointFilled className="h-3 w-3" />
            </CompositorPulse>
            {t("office:live")}
          </span>
        </TooltipTrigger>
        <TooltipContent>{t("office:agentIsActivelyWorkingOnThis")}</TooltipContent>
      </Tooltip>
    );
  }

  if (normalized === "in_review" || normalized === "review") {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <span className={cn("inline-flex items-center gap-1 text-xs text-amber-500", className)}>
            <IconPointFilled className="h-3 w-3" />
            {t("office:ready")}
          </span>
        </TooltipTrigger>
        <TooltipContent>{t("office:agentFinishedWorkspaceReadyForReview")}</TooltipContent>
      </Tooltip>
    );
  }

  return null;
}
