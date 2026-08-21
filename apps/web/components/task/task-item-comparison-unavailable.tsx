import { IconAlertTriangle } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";

export function TaskItemComparisonUnavailable({ unavailable }: { unavailable?: boolean }) {
  const { t } = useTranslation();
  if (!unavailable) return null;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          data-testid="task-comparison-unavailable-icon"
          className="inline-flex shrink-0 cursor-help text-amber-500"
          aria-label={t("task:comparisonTargetUnavailable")}
        >
          <IconAlertTriangle className="h-3.5 w-3.5" aria-hidden="true" />
        </span>
      </TooltipTrigger>
      <TooltipContent side="right">{t("task:comparisonTargetUnavailable")}</TooltipContent>
    </Tooltip>
  );
}
