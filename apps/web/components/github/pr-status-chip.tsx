"use client";

import { useRef } from "react";
import {
  IconCircleCheckFilled,
  IconCircleXFilled,
  IconChecklist,
  IconLoader2,
  IconPointFilled,
} from "@tabler/icons-react";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@kandev/ui/hover-card";
import { useTaskPR } from "@/hooks/domains/github/use-task-pr";
import { usePRFeedbackBackgroundSync } from "@/hooks/domains/github/use-pr-ci-popover";
import { PRCIPopover } from "@/components/github/pr-ci-popover";
import { isPRAwaitingReview, isPRReadyToMerge } from "@/components/github/pr-task-icon";
import type { TaskPR } from "@/lib/types/github";

const HOVER_OPEN_DELAY_MS = 150;
const HOVER_CLOSE_DELAY_MS = 150;

type ChipStatus = "passed" | "failed" | "in_progress" | "merged" | "closed" | "neutral";

function chipStatus(pr: TaskPR): ChipStatus {
  if (pr.state === "merged") return "merged";
  if (pr.state === "closed") return "closed";
  if (pr.review_state === "changes_requested" || pr.checks_state === "failure") return "failed";
  // Pending checks / pending review must beat checks_state === "success" so a
  // PR with all checks green but reviewers still outstanding renders as
  // in-progress, not passed. Without this order, the chip flips to green the
  // moment CI finishes and ignores the human gate. isPRAwaitingReview also
  // covers approved PRs where branch protection requires more reviewers.
  if (pr.checks_state === "pending" || pr.review_state === "pending") return "in_progress";
  // Mirror getPRStatusColor priority: ready-to-merge beats awaiting-review so
  // the chip and icon never disagree on a (theoretical) clean+approved+pending PR.
  if (isPRAwaitingReview(pr) && !isPRReadyToMerge(pr)) return "in_progress";
  if (pr.checks_state === "success") return "passed";
  return "neutral";
}

/**
 * Compact CI indicator for the chat status bar — a "CI" prefix icon plus a
 * status glyph that mirrors the popover's bucket colors:
 *   passed  → green check
 *   failed  → red X
 *   in progress → yellow spinner
 *   merged  → purple dot
 *   neutral → muted dot
 *
 * Hovering opens the full PRCIPopover anchored to the top edge so the card
 * expands upward (the chip sits just above the chat input).
 *
 * Returns null when the task has no PR yet.
 */
export function PRStatusChip({ taskId }: { taskId: string | null }) {
  const { pr } = useTaskPR(taskId);
  // Subscribe at the chip level so the cache warms even when the
  // top-bar PR button isn't mounted (e.g. small viewport that hides it).
  usePRFeedbackBackgroundSync(pr);
  if (!pr) return null;
  return <PRStatusChipInner pr={pr} />;
}

function PRStatusChipInner({ pr }: { pr: TaskPR }) {
  const status = chipStatus(pr);
  const triggerRef = useRef<HTMLButtonElement>(null);
  return (
    <HoverCard openDelay={HOVER_OPEN_DELAY_MS} closeDelay={HOVER_CLOSE_DELAY_MS}>
      <HoverCardTrigger asChild>
        <button
          ref={triggerRef}
          type="button"
          data-testid="pr-status-chip"
          data-pr-number={pr.pr_number}
          data-pr-state={pr.state}
          data-status={status}
          data-pr-ready-to-merge={isPRReadyToMerge(pr) ? "true" : "false"}
          aria-label={`Pull request #${pr.pr_number} CI status`}
          className="cursor-pointer inline-flex items-center gap-1 rounded-md px-1 py-0.5 text-xs"
        >
          <IconChecklist className="h-3.5 w-3.5 text-muted-foreground" aria-hidden="true" />
          <ChipStatusGlyph status={status} />
        </button>
      </HoverCardTrigger>
      <HoverCardContent
        side="top"
        align="start"
        sideOffset={8}
        className="w-80 p-2.5"
        onPointerDownOutside={(e) => {
          // Radix HoverCard treats the trigger as outside the content's
          // bounding box, so a click on the chip would auto-close the
          // popover. Filter out trigger clicks so clicking the chip is
          // a no-op while the popover stays open via hover.
          if (triggerRef.current && triggerRef.current.contains(e.target as Node)) {
            e.preventDefault();
          }
        }}
      >
        <PRCIPopover pr={pr} enabled={true} />
      </HoverCardContent>
    </HoverCard>
  );
}

function ChipStatusGlyph({ status }: { status: ChipStatus }) {
  switch (status) {
    case "passed":
      return <IconCircleCheckFilled className="h-3.5 w-3.5 text-green-500" aria-hidden="true" />;
    case "failed":
      return <IconCircleXFilled className="h-3.5 w-3.5 text-red-500" aria-hidden="true" />;
    case "in_progress":
      // CI runs take minutes, so slow the spin to ~3s/rotation — the default
      // animate-spin (1s) feels frantic for a long-running task.
      return (
        <IconLoader2
          className="h-3.5 w-3.5 text-yellow-500 animate-spin [animation-duration:3s]"
          aria-hidden="true"
        />
      );
    case "merged":
      return <IconPointFilled className="h-3.5 w-3.5 text-purple-500" aria-hidden="true" />;
    case "closed":
      return <IconPointFilled className="h-3.5 w-3.5 text-red-500" aria-hidden="true" />;
    default:
      return <IconPointFilled className="h-3.5 w-3.5 text-muted-foreground" aria-hidden="true" />;
  }
}
