"use client";

import { IconGitPullRequest } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import { useAppStore } from "@/components/state-provider";
import type { TaskPR } from "@/lib/types/github";

// Requires checks_state === "success" (not just "") so repos with no CI configured
// won't trigger ready-to-merge on mergeable_state=clean alone.
export function isPRReadyToMerge(pr: TaskPR): boolean {
  if (pr.state !== "open") return false;
  if (pr.checks_state !== "success") return false;
  if (pr.mergeable_state !== "clean") return false;
  if (pr.review_state === "approved") return true;
  // No review process: no requested reviewers and no submitted reviews. GitHub
  // sets mergeable_state=clean when branch protection is satisfied, so this
  // covers repos without required reviewers.
  return pr.review_state === "" && pr.pending_review_count === 0;
}

// CI passed but the PR is still waiting on human review (reviewers requested
// or pending review state). Distinct from yellow "CI running".
export function isPRAwaitingReview(pr: TaskPR): boolean {
  if (pr.state !== "open") return false;
  if (pr.checks_state !== "success") return false;
  if (pr.review_state === "approved") return false;
  return pr.review_state === "pending" || pr.pending_review_count > 0;
}

export function getPRStatusColor(pr: TaskPR): string {
  if (pr.state === "merged") return "text-purple-500";
  if (pr.state === "closed") return "text-red-500";
  if (pr.review_state === "changes_requested" || pr.checks_state === "failure") {
    return "text-red-500";
  }
  if (isPRReadyToMerge(pr)) {
    return "text-emerald-400";
  }
  if (pr.review_state === "approved" && pr.checks_state === "success") {
    return "text-green-500";
  }
  if (isPRAwaitingReview(pr)) {
    return "text-sky-400";
  }
  if (pr.checks_state === "pending" || pr.review_state === "pending") {
    return "text-yellow-500";
  }
  return "text-muted-foreground";
}

export function getPRTooltip(pr: TaskPR): string {
  const parts = [`PR #${pr.pr_number}: ${pr.pr_title}`];
  if (pr.state !== "open") parts.push(`State: ${pr.state}`);
  if (pr.review_state) parts.push(`Review: ${pr.review_state}`);
  if (pr.checks_state) parts.push(`CI: ${pr.checks_state}`);
  if (isPRReadyToMerge(pr)) {
    parts.push("Ready to merge");
  } else if (pr.mergeable_state && pr.mergeable_state !== "unknown" && pr.state === "open") {
    parts.push(`Mergeable: ${pr.mergeable_state}`);
  }
  return parts.join(" | ");
}

export function PRTaskIcon({ taskId }: { taskId: string }) {
  const pr = useAppStore((state) => state.taskPRs.byTaskId[taskId] ?? null);

  if (!pr) return null;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          data-testid={`pr-task-icon-${taskId}`}
          data-pr-state={pr.state}
          data-pr-ready-to-merge={isPRReadyToMerge(pr) ? "true" : "false"}
          className={cn("inline-flex items-center shrink-0", getPRStatusColor(pr))}
        >
          <IconGitPullRequest className="h-3.5 w-3.5" />
        </span>
      </TooltipTrigger>
      <TooltipContent>{getPRTooltip(pr)}</TooltipContent>
    </Tooltip>
  );
}
