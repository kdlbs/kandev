"use client";

import { IconGitPullRequest } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import { useAppStore } from "@/components/state-provider";
import type { TaskPR } from "@/lib/types/github";

const STATUS_RANK: Record<string, number> = {
  // Higher = more attention-worthy. Drives the aggregated icon color when a
  // task has multiple PRs (we surface the worst state).
  "text-red-500": 5,
  "text-yellow-500": 4,
  "text-sky-400": 3,
  "text-emerald-400": 2,
  "text-green-500": 1,
  "text-purple-500": 0,
  "text-muted-foreground": 0,
};

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
// or pending review state). Distinct from yellow "CI running". An approved
// PR with extra reviewers still pending also counts — GitHub's
// review_state="approved" only means at least one reviewer approved, not
// that branch protection's required count is met.
export function isPRAwaitingReview(pr: TaskPR): boolean {
  if (pr.state !== "open") return false;
  if (pr.checks_state !== "success") return false;
  if (pr.review_state === "approved") return pr.pending_review_count > 0;
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
  // Check awaiting-review before the plain-green fallback so an approved PR
  // with pending reviewers (1 of N required) doesn't read as fully approved.
  if (isPRAwaitingReview(pr)) {
    return "text-sky-400";
  }
  if (pr.review_state === "approved" && pr.checks_state === "success") {
    return "text-green-500";
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

/**
 * Picks the most attention-worthy color across N PRs. For multi-repo tasks one
 * red PR should dominate the visual even if the others are green.
 */
export function aggregatePRStatusColor(prs: TaskPR[]): string {
  if (prs.length === 0) return "text-muted-foreground";
  let bestColor = "text-muted-foreground";
  let bestRank = -1;
  for (const pr of prs) {
    const color = getPRStatusColor(pr);
    const rank = STATUS_RANK[color] ?? 0;
    if (rank > bestRank) {
      bestRank = rank;
      bestColor = color;
    }
  }
  return bestColor;
}

export function PRTaskIcon({ taskId }: { taskId: string }) {
  const prs = useAppStore((state) => state.taskPRs.byTaskId[taskId] ?? null);

  // Defensive: an upstream payload may briefly seed byTaskId[taskId] with a
  // non-array value (e.g. an empty object from a partial hydration). Bail
  // instead of falling through into MultiPRIcon, where for-of throws.
  if (!Array.isArray(prs) || prs.length === 0) return null;
  if (prs.length === 1) return <SinglePRIcon taskId={taskId} pr={prs[0]} />;
  return <MultiPRIcon taskId={taskId} prs={prs} />;
}

function SinglePRIcon({ taskId, pr }: { taskId: string; pr: TaskPR }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          data-testid={`pr-task-icon-${taskId}`}
          data-pr-state={pr.state}
          data-pr-count="1"
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

function MultiPRIcon({ taskId, prs }: { taskId: string; prs: TaskPR[] }) {
  const aggregateColor = aggregatePRStatusColor(prs);
  const allReady = prs.every(isPRReadyToMerge);
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          data-testid={`pr-task-icon-${taskId}`}
          data-pr-count={prs.length}
          data-pr-ready-to-merge={allReady ? "true" : "false"}
          className={cn("inline-flex items-center gap-0.5 shrink-0", aggregateColor)}
        >
          <IconGitPullRequest className="h-3.5 w-3.5" />
          <span className="text-[9px] font-semibold leading-none">{prs.length}</span>
        </span>
      </TooltipTrigger>
      <TooltipContent>
        <div className="flex flex-col gap-1 text-xs">
          {prs.map((pr) => (
            <div key={pr.id} className="flex items-center gap-2">
              <span className={cn("inline-flex shrink-0", getPRStatusColor(pr))}>
                <IconGitPullRequest className="h-3 w-3" />
              </span>
              <span>{getPRTooltip(pr)}</span>
            </div>
          ))}
        </div>
      </TooltipContent>
    </Tooltip>
  );
}
