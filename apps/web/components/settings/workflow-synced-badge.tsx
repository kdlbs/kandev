import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";

export function WorkflowSyncedBadge({ sourcePath }: { sourcePath?: string }) {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge
          variant="outline"
          tabIndex={0}
          className="text-xs cursor-default"
          data-testid="workflow-synced-badge"
        >
          {t("workflows:synced")}
        </Badge>
      </TooltipTrigger>
      <TooltipContent>
        {t("workflows:syncedBadgeTooltip", {
          source: sourcePath || t("workflows:aConfiguredRepository"),
        })}
      </TooltipContent>
    </Tooltip>
  );
}
