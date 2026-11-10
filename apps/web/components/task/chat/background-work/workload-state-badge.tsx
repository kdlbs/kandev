import { Badge } from "@kandev/ui/badge";
import { useTranslation } from "react-i18next";
import type { WorkloadRunObservation } from "@/lib/types/background-work";
import { cn } from "@/lib/utils";
import { workloadStateBadgeVariant, workloadStateLabelKey } from "./presentation";

/** Shared run-state badge for the chip drawer and the desktop detail panel. */
export function WorkloadStateBadge({ workload }: { workload: WorkloadRunObservation }) {
  const { t } = useTranslation();
  return (
    <Badge
      variant={workloadStateBadgeVariant(workload.state)}
      className={cn(
        "text-[10px]",
        workload.state === "running" && "bg-primary text-primary-foreground",
      )}
      data-testid={`background-work-state-${workload.work_id}`}
      data-state={workload.state}
    >
      {t(workloadStateLabelKey(workload.state))}
    </Badge>
  );
}
