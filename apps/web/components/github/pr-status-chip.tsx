"use client";

import { HoverCard, HoverCardContent, HoverCardTrigger } from "@kandev/ui/hover-card";
import { useTaskPR } from "@/hooks/domains/github/use-task-pr";
import { usePRFeedbackBackgroundSync } from "@/hooks/domains/github/use-pr-ci-popover";
import { PRCIPopover } from "@/components/github/pr-ci-popover";
import { getPRStatusColor, isPRReadyToMerge } from "@/components/github/pr-task-icon";
import type { TaskPR } from "@/lib/types/github";

const HOVER_OPEN_DELAY_MS = 150;
const HOVER_CLOSE_DELAY_MS = 150;

/**
 * Compact PR-status indicator for the chat status bar — a small colored
 * circle that opens the same CI popover as the top-bar button on hover.
 * The popover anchors to the top of the chip so it expands upward (the
 * chip lives near the bottom of the chat panel, just above the input).
 *
 * Returns null when the task has no PR yet, so the chip simply doesn't
 * show until a PR is associated.
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
  // Reuse getPRStatusColor's text-* class on the wrapper and let an SVG
  // circle pick up `currentColor` for its fill. The text-* utilities are
  // already compiled (used elsewhere by PRTaskIcon), so we sidestep any
  // bg-* variant that might be missing and the chip is guaranteed
  // to render with colour.
  return (
    <HoverCard openDelay={HOVER_OPEN_DELAY_MS} closeDelay={HOVER_CLOSE_DELAY_MS}>
      <HoverCardTrigger asChild>
        <button
          type="button"
          data-testid="pr-status-chip"
          data-pr-number={pr.pr_number}
          data-pr-state={pr.state}
          data-pr-ready-to-merge={isPRReadyToMerge(pr) ? "true" : "false"}
          aria-label={`Pull request #${pr.pr_number} CI status`}
          className={`cursor-pointer inline-flex items-center justify-center rounded-full p-0.5 hover:bg-accent/50 ${getPRStatusColor(pr)}`}
        >
          <svg viewBox="0 0 10 10" className="h-2.5 w-2.5" aria-hidden="true" focusable="false">
            <circle cx="5" cy="5" r="5" fill="currentColor" />
          </svg>
        </button>
      </HoverCardTrigger>
      <HoverCardContent side="top" align="start" sideOffset={8} className="w-80 p-2.5">
        <PRCIPopover pr={pr} enabled={true} />
      </HoverCardContent>
    </HoverCard>
  );
}
