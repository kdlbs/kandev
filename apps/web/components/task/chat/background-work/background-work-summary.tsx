"use client";

import { IconExternalLink, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import type { WorkloadRunObservation } from "@/lib/types/background-work";
import { cn } from "@/lib/utils";
import { workloadKindLabelKey } from "./presentation";
import { WorkloadStateBadge } from "./workload-state-badge";

type RunningWorkSummaryProps = {
  workloads: WorkloadRunObservation[];
  onOpenWorkload: (workId: string, title: string) => void;
  onOpenAll: () => void;
  onClose?: () => void;
  drawer?: boolean;
};

/**
 * Composer summary for accepted workloads. Membership is the running subset
 * only; terminal, waiting, and unknown records stay in the overview history.
 */
export function RunningWorkSummary({
  workloads,
  onOpenWorkload,
  onOpenAll,
  onClose,
  drawer = false,
}: RunningWorkSummaryProps) {
  const { t } = useTranslation();
  return (
    <div
      data-testid="background-work-summary-list"
      className={cn("flex min-w-0 flex-col gap-2", drawer && "min-h-0 overflow-y-auto p-3")}
      data-vaul-no-drag={drawer ? true : undefined}
    >
      <div className="flex min-w-0 items-center justify-between gap-2 border-b border-border pb-2">
        <span className="min-w-0 truncate text-xs font-semibold text-foreground">
          {t("task:backgroundRunningTitle", { count: workloads.length })}
        </span>
        {onClose && (
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            onClick={onClose}
            aria-label={t("task:backgroundWorkClose")}
            className={cn("shrink-0 cursor-pointer", drawer && "min-h-11 min-w-11")}
            data-testid="background-work-summary-close"
          >
            <IconX aria-hidden="true" />
          </Button>
        )}
      </div>
      <div className="flex max-h-56 min-w-0 flex-col gap-1.5 overflow-y-auto">
        {workloads.map((workload) => (
          <div
            key={workload.work_id}
            data-testid={`background-work-summary-item-${workload.work_id}`}
            className="flex min-w-0 items-center justify-between gap-2 rounded border border-border/60 bg-muted/30 px-2 py-1.5 text-xs"
          >
            <div className="flex min-w-0 flex-1 flex-col">
              <span
                className="min-w-0 truncate font-medium text-foreground"
                title={workload.title}
                data-testid={`background-work-summary-title-${workload.work_id}`}
              >
                {workload.title}
              </span>
              <span className="truncate text-[10px] text-muted-foreground">
                {t(workloadKindLabelKey(workload.kind))}
              </span>
            </div>
            <div className="flex shrink-0 items-center gap-1.5">
              <WorkloadStateBadge workload={workload} />
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className={cn(
                  "cursor-pointer px-2 text-[11px]",
                  drawer ? "min-h-11 min-w-11" : "h-6",
                )}
                onClick={() => onOpenWorkload(workload.work_id, workload.title)}
                aria-label={`${t("task:openBackgroundWork")} ${workload.title}`}
                data-testid={`background-work-open-button-${workload.work_id}`}
              >
                {t("task:openBackgroundWork")}
                <IconExternalLink className="ml-1 h-3 w-3" />
              </Button>
            </div>
          </div>
        ))}
      </div>
      <div className="pt-1">
        <Button
          type="button"
          variant="outline"
          size="sm"
          className={cn("w-full cursor-pointer justify-center text-xs", drawer && "min-h-11")}
          onClick={onOpenAll}
          data-testid="background-work-view-all-button"
        >
          {t("task:viewAllBackgroundWork")}
        </Button>
      </div>
    </div>
  );
}
