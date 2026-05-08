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
          className="cursor-pointer rounded-full p-0.5 hover:bg-accent/50"
        >
          <span className={`block h-2.5 w-2.5 rounded-full ${chipBg(pr)}`} aria-hidden />
        </button>
      </HoverCardTrigger>
      <HoverCardContent side="top" align="start" sideOffset={8} className="w-80 p-2.5">
        <PRCIPopover pr={pr} enabled={true} />
      </HoverCardContent>
    </HoverCard>
  );
}

/**
 * Background colour token mirroring `getPRStatusColor` (which returns a
 * `text-*` class). Doing the mapping inline keeps the chip's styling
 * (`bg-*` on a filled circle) consistent with the icon colour without
 * round-tripping through a foreground token.
 */
function chipBg(pr: TaskPR): string {
  // Reuse the canonical mapping; the text-class string is stable so we
  // can swap "text-" → "bg-" and pick up the same colour.
  return getPRStatusColor(pr).replace(/\btext-/g, "bg-");
}
