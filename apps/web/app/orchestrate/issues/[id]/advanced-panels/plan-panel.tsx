"use client";

import { IconListDetails } from "@tabler/icons-react";

type AdvancedPlanPanelProps = {
  taskId: string;
};

export function AdvancedPlanPanel({ taskId: _taskId }: AdvancedPlanPanelProps) {
  return (
    <div className="flex flex-col items-center justify-center h-full bg-muted/20">
      <IconListDetails className="h-8 w-8 text-muted-foreground/40 mb-3" />
      <p className="text-sm text-muted-foreground">Plan</p>
      <p className="text-xs text-muted-foreground mt-1">
        The execution plan will appear here once the agent begins working.
      </p>
    </div>
  );
}
