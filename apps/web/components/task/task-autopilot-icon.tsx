"use client";

import { IconRobot } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";

export function TaskAutopilotIcon() {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          data-testid="task-autopilot-icon"
          className="inline-flex shrink-0 cursor-help text-yellow-500"
          aria-label={t("task:autopilotSidebarDescription")}
        >
          <IconRobot className="h-3.5 w-3.5" aria-hidden="true" />
        </span>
      </TooltipTrigger>
      <TooltipContent side="right">{t("task:autopilotSidebarDescription")}</TooltipContent>
    </Tooltip>
  );
}
