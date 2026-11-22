import { useTranslation } from "react-i18next";
import { IconRefresh } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";

// PromptSyncedBadge marks a prompt owned by Prompt Sync. Its tooltip names the
// repository file; the prompt is edited there, not in Kandev.
export function PromptSyncedBadge({ sourcePath }: { sourcePath?: string }) {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge
          variant="outline"
          tabIndex={0}
          className="text-xs cursor-default"
          data-testid="prompt-synced-badge"
        >
          <IconRefresh className="h-3 w-3 mr-1" />
          {t("settings:promptSynced")}
        </Badge>
      </TooltipTrigger>
      <TooltipContent>
        {t("settings:promptSyncedTooltip", { path: sourcePath ?? "" })}
      </TooltipContent>
    </Tooltip>
  );
}
