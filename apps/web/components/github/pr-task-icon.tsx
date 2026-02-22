"use client";

import { IconGitPullRequest } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import { useAppStore } from "@/components/state-provider";
import type { TaskPR } from "@/lib/types/github";

function getPRStatusColor(pr: TaskPR): string {
  if (pr.state === "merged") return "text-purple-500";
  if (pr.state === "closed") return "text-red-500";
  if (pr.review_state === "changes_requested" || pr.checks_state === "failure") {
    return "text-red-500";
  }
  if (pr.review_state === "approved" && pr.checks_state === "success") {
    return "text-green-500";
  }
  if (pr.checks_state === "pending" || pr.review_state === "pending") {
    return "text-yellow-500";
  }
  return "text-muted-foreground";
}

function getPRTooltip(pr: TaskPR): string {
  const parts = [`PR #${pr.pr_number}: ${pr.pr_title}`];
  if (pr.state !== "open") parts.push(`State: ${pr.state}`);
  if (pr.review_state) parts.push(`Review: ${pr.review_state}`);
  if (pr.checks_state) parts.push(`CI: ${pr.checks_state}`);
  return parts.join(" | ");
}

export function PRTaskIcon({ taskId }: { taskId: string }) {
  const pr = useAppStore((state) => state.taskPRs.byTaskId[taskId] ?? null);

  if (!pr) return null;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className={cn("inline-flex items-center", getPRStatusColor(pr))}>
          <IconGitPullRequest className="h-3.5 w-3.5" />
        </span>
      </TooltipTrigger>
      <TooltipContent>{getPRTooltip(pr)}</TooltipContent>
    </Tooltip>
  );
}
