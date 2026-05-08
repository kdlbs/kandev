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
 * Background colour mapping that mirrors `getPRStatusColor` (which returns
 * a `text-*` class). Tailwind's content scanner only sees literal class
 * strings, so we keep this as a static lookup instead of building it via
 * string replacement at runtime — otherwise the bundle drops the bg-*
 * styles and the chip renders without colour.
 */
const CHIP_BG_BY_TEXT_CLASS: Record<string, string> = {
  "text-red-500": "bg-red-500",
  "text-yellow-500": "bg-yellow-500",
  "text-sky-400": "bg-sky-400",
  "text-emerald-400": "bg-emerald-400",
  "text-green-500": "bg-green-500",
  "text-purple-500": "bg-purple-500",
  "text-muted-foreground": "bg-muted-foreground",
};

function chipBg(pr: TaskPR): string {
  return CHIP_BG_BY_TEXT_CLASS[getPRStatusColor(pr)] ?? "bg-muted-foreground";
}
