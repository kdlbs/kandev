"use client";

import { memo } from "react";
import {
  IconGitPullRequest,
  IconCheck,
  IconX,
  IconClock,
  IconChevronDown,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useTaskPR } from "@/hooks/domains/github/use-task-pr";
import {
  aggregatePRStatusColor,
  getPRStatusColor,
  isPRReadyToMerge,
} from "@/components/github/pr-task-icon";
import { prTaskKey } from "@/components/github/pr-detail-panel";
import { useAppStore } from "@/components/state-provider";
import type { TaskPR } from "@/lib/types/github";

function PRStatusIcon({ pr }: { pr: TaskPR }) {
  // Terminal states take priority
  if (pr.state === "merged") {
    return <IconCheck className="h-3 w-3 text-purple-500" />;
  }
  if (pr.state === "closed") {
    return <IconX className="h-3 w-3 text-muted-foreground" />;
  }
  // Review/check states only matter for open PRs
  if (pr.checks_state === "failure" || pr.review_state === "changes_requested") {
    return <IconX className="h-3 w-3 text-red-500" />;
  }
  if (isPRReadyToMerge(pr)) {
    return <IconCheck className="h-3 w-3 text-emerald-400" />;
  }
  if (pr.checks_state === "success" && pr.review_state === "approved") {
    return <IconCheck className="h-3 w-3 text-green-500" />;
  }
  if (pr.checks_state === "pending" || pr.review_state === "pending") {
    return <IconClock className="h-3 w-3 text-yellow-500" />;
  }
  return null;
}

export const PRTopbarButton = memo(function PRTopbarButton() {
  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  // useTaskPR fetches if not in store and returns the full per-task list so
  // multi-repo tasks can surface every PR (one button for single-repo, a
  // dropdown summary for 2+ so the topbar doesn't blow up horizontally).
  const { prs } = useTaskPR(activeTaskId);

  if (prs.length === 0) return null;
  if (prs.length === 1) return <PRSingleButton pr={prs[0]} />;
  return <PRMultiButton prs={prs} />;
});

function PRSingleButton({ pr }: { pr: TaskPR }) {
  const addPRPanel = useDockviewStore((s) => s.addPRPanel);
  const tooltip = `${pr.owner}/${pr.repo} #${pr.pr_number} — ${pr.pr_title}`;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          data-testid="pr-topbar-button"
          data-pr-number={pr.pr_number}
          data-pr-state={pr.state}
          data-pr-ready-to-merge={isPRReadyToMerge(pr) ? "true" : "false"}
          size="sm"
          variant="outline"
          className="cursor-pointer gap-1.5 px-2"
          onClick={() => addPRPanel(prTaskKey(pr))}
        >
          <IconGitPullRequest className={`h-4 w-4 ${getPRStatusColor(pr)}`} />
          <span className="text-xs font-medium">#{pr.pr_number}</span>
          <PRStatusIcon pr={pr} />
        </Button>
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}

function PRMultiButton({ prs }: { prs: TaskPR[] }) {
  const addPRPanel = useDockviewStore((s) => s.addPRPanel);
  const aggColor = aggregatePRStatusColor(prs);
  return (
    <DropdownMenu>
      <Tooltip>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <Button
              data-testid="pr-topbar-button"
              data-pr-count={prs.length}
              size="sm"
              variant="outline"
              className="cursor-pointer gap-1.5 px-2"
            >
              <IconGitPullRequest className={`h-4 w-4 ${aggColor}`} />
              <span className="text-xs font-medium">{prs.length} PRs</span>
              <IconChevronDown className="h-3 w-3 text-muted-foreground" />
            </Button>
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent>{prs.length} pull requests linked to this task — open one</TooltipContent>
      </Tooltip>
      <DropdownMenuContent align="end" className="w-72">
        <DropdownMenuLabel className="text-xs">Pull requests</DropdownMenuLabel>
        <DropdownMenuSeparator />
        {prs.map((pr) => (
          <DropdownMenuItem
            key={pr.id}
            onClick={() => addPRPanel(prTaskKey(pr))}
            className="cursor-pointer gap-2"
            data-testid={`pr-topbar-menu-item-${pr.pr_number}`}
          >
            <IconGitPullRequest className={`h-4 w-4 shrink-0 ${getPRStatusColor(pr)}`} />
            <div className="flex flex-col min-w-0 flex-1">
              <span className="text-xs font-medium">
                {pr.repo} #{pr.pr_number}
              </span>
              <span className="text-[11px] text-muted-foreground truncate">{pr.pr_title}</span>
            </div>
            <PRStatusIcon pr={pr} />
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
