"use client";

import { useTranslation } from "react-i18next";
import { IconHierarchy3 } from "@tabler/icons-react";

import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useOpenTaskGraph } from "@/hooks/domains/task/use-open-task-graph";
import { useTaskGraphAvailable } from "@/hooks/domains/task/use-task-graph-available";

// Matches the icon-only Share and auto-scroll controls so the graph sits flush
// with the right utility group; phones and coarse pointers get a 44px target.
const TASK_GRAPH_BUTTON_CLASS =
  "h-6 w-6 max-md:h-11 max-md:w-11 [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11 p-0 cursor-pointer text-muted-foreground hover:bg-muted/70 hover:text-foreground focus-visible:ring-1 focus-visible:ring-ring";

/** Right-utility entry point; renders nothing when the task has no relations. */
export function TaskGraphButton({ taskId }: { taskId: string | null }) {
  const { t } = useTranslation();
  const available = useTaskGraphAvailable(taskId);
  const { openTab } = useOpenTaskGraph();
  if (!taskId || !available) return null;
  const label = t("task:taskGraphTitle");
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => openTab(taskId)}
          className={TASK_GRAPH_BUTTON_CLASS}
          aria-label={label}
          data-testid="task-graph-button"
        >
          <IconHierarchy3 aria-hidden="true" className="h-3.5 w-3.5" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}
