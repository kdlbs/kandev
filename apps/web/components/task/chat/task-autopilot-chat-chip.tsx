"use client";

import { IconRobot } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";

export function useTaskAutopilot(taskId: string | null): boolean {
  return useAppStore((state) => {
    if (!taskId) return false;
    const direct = state.kanban.tasks.find((task) => task.id === taskId);
    if (direct?.autopilot === true) return true;
    if (direct?.autopilot === false) return false;
    return Object.values(state.kanbanMulti.snapshots).some((snapshot) =>
      snapshot.tasks.some((task) => task.id === taskId && task.autopilot === true),
    );
  });
}

export function AutopilotChatChip() {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          data-testid="chat-autopilot-chip"
          className="inline-flex items-center gap-1 rounded-full border border-yellow-500/40 bg-yellow-500/10 px-2 py-0.5 text-[11px] font-medium text-yellow-600 dark:text-yellow-400"
          aria-label={t("task:autopilotChatDescription")}
        >
          <IconRobot className="h-3 w-3" aria-hidden="true" />
          {t("task:autopilot")}
        </span>
      </TooltipTrigger>
      <TooltipContent>{t("task:autopilotChatDescription")}</TooltipContent>
    </Tooltip>
  );
}
