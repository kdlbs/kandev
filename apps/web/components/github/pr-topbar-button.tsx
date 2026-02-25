"use client";

import { memo } from "react";
import { IconGitPullRequest, IconCheck, IconX, IconClock } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useActiveTaskPR, useTaskPR } from "@/hooks/domains/github/use-task-pr";
import { useAppStore } from "@/components/state-provider";
import type { TaskPR } from "@/lib/types/github";

function prIconColor(state: string): string {
  if (state === "merged") return "text-purple-500";
  if (state === "closed") return "text-red-500";
  return "text-green-600";
}

function PRStatusIcon({ pr }: { pr: TaskPR }) {
  if (pr.checks_state === "failure" || pr.review_state === "changes_requested") {
    return <IconX className="h-3 w-3 text-red-500" />;
  }
  if (pr.checks_state === "success" && pr.review_state === "approved") {
    return <IconCheck className="h-3 w-3 text-green-500" />;
  }
  if (pr.checks_state === "pending" || pr.review_state === "pending") {
    return <IconClock className="h-3 w-3 text-yellow-500" />;
  }
  if (pr.state === "merged") {
    return <IconCheck className="h-3 w-3 text-purple-500" />;
  }
  if (pr.state === "closed") {
    return <IconX className="h-3 w-3 text-muted-foreground" />;
  }
  return null;
}

export const PRTopbarButton = memo(function PRTopbarButton() {
  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  // useTaskPR fetches if not in store; useActiveTaskPR just reads
  useTaskPR(activeTaskId);
  const pr = useActiveTaskPR();
  const addPRPanel = useDockviewStore((s) => s.addPRPanel);

  if (!pr) return null;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          size="sm"
          variant="outline"
          className="cursor-pointer gap-1.5 px-2"
          onClick={addPRPanel}
        >
          <IconGitPullRequest className={`h-4 w-4 ${prIconColor(pr.state)}`} />
          <span className="text-xs font-medium">#{pr.pr_number}</span>
          <PRStatusIcon pr={pr} />
        </Button>
      </TooltipTrigger>
      <TooltipContent>{pr.pr_title}</TooltipContent>
    </Tooltip>
  );
});
