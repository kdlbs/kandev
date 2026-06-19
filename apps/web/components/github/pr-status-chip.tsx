"use client";

import { useCallback, useRef, useState } from "react";
import {
  IconCircleCheckFilled,
  IconCircleXFilled,
  IconChecklist,
  IconLoader2,
  IconPointFilled,
  IconAlertTriangleFilled,
  IconX,
} from "@tabler/icons-react";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@kandev/ui/hover-card";
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { Button } from "@kandev/ui/button";
import { useTaskPR } from "@/hooks/domains/github/use-task-pr";
import { usePRFeedbackBackgroundSync } from "@/hooks/domains/github/use-pr-ci-popover";
import { PR_CI_DESKTOP_POPOVER_SCROLL_CLASS, PRCIPopover } from "@/components/github/pr-ci-popover";
import { useTaskCIAutomationOptions } from "@/hooks/domains/github/use-task-ci-options";
import { MultiPRCIPopover } from "@/components/github/multi-pr-ci-popover";
import {
  isPRAwaitingReview,
  isPRReadyToMerge,
  pickDefaultPR,
} from "@/components/github/pr-task-icon";
import { useIsMobile } from "@/hooks/use-mobile";
import type { TaskPR } from "@/lib/types/github";

const HOVER_OPEN_DELAY_MS = 150;
const HOVER_CLOSE_DELAY_MS = 150;

// Terminal states (merged / closed) never reach here — PRStatusChip returns
// null for them before rendering — so the chip status union omits them.
type ChipStatus =
	| "passed"
	| "failed"
	| "conflict"
	| "blocked"
	| "behind"
	| "in_progress"
	| "neutral";
type AutomationFlags = {
	autoFix: boolean;
	autoMerge: boolean;
};

function chipStatus(pr: TaskPR): ChipStatus {
  if (pr.review_state === "changes_requested" || pr.checks_state === "failure") return "failed";
  // Merge conflicts / behind-base block the merge even when CI is green — the
  // chip must never read as a passed check in that case. Mirrors
  // getPRStatusColor + PRStatusIcon (dirty = red, behind = amber).
  if (pr.mergeable_state === "dirty") return "conflict";
  if (pr.mergeable_state === "behind") return "behind";
  // Pending checks / pending review must beat checks_state === "success" so a
  // PR with all checks green but reviewers still outstanding renders as
  // in-progress, not passed. Without this order, the chip flips to green the
  // moment CI finishes and ignores the human gate. isPRAwaitingReview also
  // covers approved PRs where branch protection requires more reviewers.
  if (pr.checks_state === "pending" || pr.review_state === "pending") return "in_progress";
  // Mirror getPRStatusColor priority: ready-to-merge beats awaiting-review so
  // the chip and icon never disagree on a (theoretical) clean+approved+pending PR.
  if (isPRAwaitingReview(pr) && !isPRReadyToMerge(pr)) return "in_progress";
  // Blocked-by-branch-protection that isn't just an outstanding review (those
  // are caught above) is a real gate — amber, not the green "passed" check.
  if (pr.mergeable_state === "blocked") return "blocked";
  if (pr.checks_state === "success") return "passed";
  return "neutral";
}

// Higher = more attention-worthy. Drives the aggregate glyph when a task has
// multiple open PRs — one failing/conflicting PR colours the whole chip.
const CHIP_STATUS_RANK: Record<ChipStatus, number> = {
  failed: 6,
  conflict: 5,
  blocked: 4,
  behind: 3,
  in_progress: 2,
  passed: 1,
  neutral: 0,
};

export function aggregateChipStatus(prs: TaskPR[]): ChipStatus {
  let worst: ChipStatus = "neutral";
  for (const pr of prs) {
    const status = chipStatus(pr);
    if (CHIP_STATUS_RANK[status] > CHIP_STATUS_RANK[worst]) worst = status;
  }
  return worst;
}

const CHIP_BUTTON_CLASS =
  "cursor-pointer inline-flex items-center gap-1 rounded-md px-1 py-0.5 text-xs";

/**
 * Radix HoverCard treats the trigger as outside the content's bounding box, so
 * a click on the chip would auto-close the popover. This guard filters out
 * trigger clicks so clicking the chip is a no-op while the popover stays open
 * via hover. Returns the trigger ref plus a memoised handler that reads the ref
 * lazily (inside the callback, never during render).
 */
function useChipTriggerGuard() {
  const ref = useRef<HTMLButtonElement>(null);
  const onPointerDownOutside = useCallback(
    (e: { target: EventTarget | null; preventDefault: () => void }) => {
      if (ref.current && ref.current.contains(e.target as Node)) {
        e.preventDefault();
      }
    },
    [],
  );
  return { ref, onPointerDownOutside };
}

/**
 * Compact CI indicator for the chat status bar — a "CI" prefix icon plus a
 * status glyph that mirrors the popover's bucket colors:
 *   passed  → green check
 *   failed  → red X
 *   in progress → yellow spinner
 *   neutral → muted dot
 *
 * Desktop: hovering opens the full PRCIPopover anchored to the top edge so the
 * card expands upward (the chip sits just above the chat input).
 *
 * Mobile: tapping opens the same popover content inside a bottom-sheet Drawer
 * — hover is unreachable on touch devices.
 *
 * Returns null when the task has no PR yet, or once the PR reaches a terminal
 * state (merged / closed) — the chat-input banner already conveys that, so the
 * CI chip would be redundant.
 */
export function PRStatusChip({ taskId }: { taskId: string | null }) {
  const { prs } = useTaskPR(taskId);
  const { options: automationOptions } = useTaskCIAutomationOptions(taskId);
  const automationFlags: AutomationFlags = {
    autoFix: Boolean(automationOptions?.auto_fix_enabled),
    autoMerge: Boolean(automationOptions?.auto_merge_enabled),
  };
  // Defensive Array.isArray: a partial hydration can briefly seed the store
  // with a non-array value (same guard as PRTaskIcon).
  // Only open PRs are worth a CI chip — terminal PRs (merged/closed) are
  // already conveyed by the chat-input banner. With multiple PRs the chip
  // stays visible as long as at least one is still open.
  const openPRs = Array.isArray(prs)
    ? prs.filter((p) => p.state !== "merged" && p.state !== "closed")
    : [];
  // Subscribe at the chip level so the cache warms even when the top-bar PR
  // button isn't mounted (e.g. small viewport that hides it). Warm the PR the
  // popover will actually open first (worst-status via pickDefaultPR — for a
  // single PR that's just the PR itself); the remaining PRs in a multi-PR
  // task warm when the popover opens.
  usePRFeedbackBackgroundSync(pickDefaultPR(openPRs));
  if (openPRs.length === 0) return null;
  if (openPRs.length === 1)
    return <PRStatusChipInner pr={openPRs[0]} automation={automationFlags} />;
  return <PRStatusChipMultiInner prs={openPRs} automation={automationFlags} />;
}

type ChipButtonAttrs = {
  "data-testid": "pr-status-chip";
  "data-pr-number": number;
  "data-pr-state": string;
  "data-status": ChipStatus;
  "data-pr-ready-to-merge": "true" | "false";
  "aria-label": string;
  className: string;
};

function chipButtonAttrs(pr: TaskPR, status: ChipStatus): ChipButtonAttrs {
  return {
    "data-testid": "pr-status-chip",
    "data-pr-number": pr.pr_number,
    "data-pr-state": pr.state,
    "data-status": status,
    "data-pr-ready-to-merge": isPRReadyToMerge(pr) ? "true" : "false",
    "aria-label": `Pull request #${pr.pr_number} CI status`,
    className: CHIP_BUTTON_CLASS,
  };
}

function AutomationFlagBadges({ automation }: { automation: AutomationFlags }) {
  if (!automation.autoFix && !automation.autoMerge) return null;
  return (
    <>
      {automation.autoFix && (
        <span
          data-testid="pr-status-auto-fix-chip"
          className="rounded-sm bg-emerald-500/15 px-1 py-0.5 text-[9px] font-medium leading-none text-emerald-500"
        >
          Auto-fix
        </span>
      )}
      {automation.autoMerge && (
        <span
          data-testid="pr-status-auto-merge-chip"
          className="rounded-sm bg-sky-500/15 px-1 py-0.5 text-[9px] font-medium leading-none text-sky-500"
        >
          Auto-merge
        </span>
      )}
    </>
  );
}

function PRStatusChipInner({ pr, automation }: { pr: TaskPR; automation: AutomationFlags }) {
  const isMobile = useIsMobile();
  if (isMobile) return <PRStatusChipDrawer pr={pr} automation={automation} />;
  return <PRStatusChipHoverCard pr={pr} automation={automation} />;
}

function PRStatusChipHoverCard({ pr, automation }: { pr: TaskPR; automation: AutomationFlags }) {
  const status = chipStatus(pr);
  const { ref, onPointerDownOutside } = useChipTriggerGuard();
  return (
    <HoverCard openDelay={HOVER_OPEN_DELAY_MS} closeDelay={HOVER_CLOSE_DELAY_MS}>
      <HoverCardTrigger asChild>
        <button ref={ref} type="button" {...chipButtonAttrs(pr, status)}>
          <IconChecklist className="h-3.5 w-3.5 text-muted-foreground" aria-hidden="true" />
          <ChipStatusGlyph status={status} />
          <AutomationFlagBadges automation={automation} />
        </button>
      </HoverCardTrigger>
      <HoverCardContent
        side="top"
        align="start"
        sideOffset={8}
        className={`w-80 p-2.5 ${PR_CI_DESKTOP_POPOVER_SCROLL_CLASS}`}
        onPointerDownOutside={onPointerDownOutside}
      >
        <PRCIPopover pr={pr} enabled={true} />
      </HoverCardContent>
    </HoverCard>
  );
}

function PRStatusChipMultiInner({
  prs,
  automation,
}: {
  prs: TaskPR[];
  automation: AutomationFlags;
}) {
  const isMobile = useIsMobile();
  if (isMobile) return <PRStatusChipMultiDrawer prs={prs} automation={automation} />;
  return <PRStatusChipMultiHoverCard prs={prs} automation={automation} />;
}

type MultiChipButtonAttrs = {
  "data-testid": "pr-status-chip";
  "data-pr-count": number;
  "data-status": ChipStatus;
  "aria-label": string;
  className: string;
};

function multiChipButtonAttrs(prs: TaskPR[], status: ChipStatus): MultiChipButtonAttrs {
  return {
    "data-testid": "pr-status-chip",
    "data-pr-count": prs.length,
    "data-status": status,
    "aria-label": `${prs.length} pull requests CI status`,
    className: CHIP_BUTTON_CLASS,
  };
}

function MultiChipGlyph({
  prs,
  status,
  automation,
}: {
  prs: TaskPR[];
  status: ChipStatus;
  automation: AutomationFlags;
}) {
  return (
    <>
      <IconChecklist className="h-3.5 w-3.5 text-muted-foreground" aria-hidden="true" />
      <ChipStatusGlyph status={status} />
      <span className="text-[9px] font-semibold leading-none tabular-nums">{prs.length}</span>
      <AutomationFlagBadges automation={automation} />
    </>
  );
}

function PRStatusChipMultiHoverCard({
  prs,
  automation,
}: {
  prs: TaskPR[];
  automation: AutomationFlags;
}) {
  const status = aggregateChipStatus(prs);
  const { ref, onPointerDownOutside } = useChipTriggerGuard();
  return (
    <HoverCard openDelay={HOVER_OPEN_DELAY_MS} closeDelay={HOVER_CLOSE_DELAY_MS}>
      <HoverCardTrigger asChild>
        <button ref={ref} type="button" {...multiChipButtonAttrs(prs, status)}>
          <MultiChipGlyph prs={prs} status={status} automation={automation} />
        </button>
      </HoverCardTrigger>
      <HoverCardContent
        side="top"
        align="start"
        sideOffset={8}
        className={`w-96 p-2.5 ${PR_CI_DESKTOP_POPOVER_SCROLL_CLASS}`}
        onPointerDownOutside={onPointerDownOutside}
      >
        <MultiPRCIPopover prs={prs} enabled={true} />
      </HoverCardContent>
    </HoverCard>
  );
}

function PRStatusChipMultiDrawer({
  prs,
  automation,
}: {
  prs: TaskPR[];
  automation: AutomationFlags;
}) {
  const status = aggregateChipStatus(prs);
  const [open, setOpen] = useState(false);
  return (
    <Drawer open={open} onOpenChange={setOpen}>
      <button
        type="button"
        aria-haspopup="dialog"
        aria-expanded={open}
        onClick={() => setOpen(true)}
        {...multiChipButtonAttrs(prs, status)}
      >
        <MultiChipGlyph prs={prs} status={status} automation={automation} />
      </button>
      <DrawerContent data-testid="pr-status-chip-drawer" className="max-h-[80vh] flex flex-col">
        <DrawerHeader className="flex flex-row items-center justify-between border-b py-2">
          <DrawerTitle className="text-sm">{prs.length} pull requests</DrawerTitle>
          <DrawerDescription className="sr-only">
            Pull request CI status, reviews, and checks summary.
          </DrawerDescription>
          <DrawerClose asChild>
            <Button
              data-testid="pr-status-chip-drawer-close"
              variant="ghost"
              size="icon-sm"
              aria-label="Close PR status"
              className="cursor-pointer"
            >
              <IconX className="h-4 w-4" />
            </Button>
          </DrawerClose>
        </DrawerHeader>
        <div className="flex-1 min-h-0 overflow-y-auto p-3" data-vaul-no-drag>
          <MultiPRCIPopover prs={prs} enabled={open} />
        </div>
      </DrawerContent>
    </Drawer>
  );
}

function PRStatusChipDrawer({ pr, automation }: { pr: TaskPR; automation: AutomationFlags }) {
  const status = chipStatus(pr);
  const [open, setOpen] = useState(false);
  return (
    <Drawer open={open} onOpenChange={setOpen}>
      <button
        type="button"
        aria-haspopup="dialog"
        aria-expanded={open}
        onClick={() => setOpen(true)}
        {...chipButtonAttrs(pr, status)}
      >
        <IconChecklist className="h-3.5 w-3.5 text-muted-foreground" aria-hidden="true" />
        <ChipStatusGlyph status={status} />
        <AutomationFlagBadges automation={automation} />
      </button>
      <DrawerContent data-testid="pr-status-chip-drawer" className="max-h-[80vh] flex flex-col">
        <DrawerHeader className="flex flex-row items-center justify-between border-b py-2">
          <DrawerTitle className="text-sm">PR #{pr.pr_number}</DrawerTitle>
          <DrawerDescription className="sr-only">
            Pull request CI status, reviews, and checks summary.
          </DrawerDescription>
          <DrawerClose asChild>
            <Button
              data-testid="pr-status-chip-drawer-close"
              variant="ghost"
              size="icon-sm"
              aria-label="Close PR status"
              className="cursor-pointer"
            >
              <IconX className="h-4 w-4" />
            </Button>
          </DrawerClose>
        </DrawerHeader>
        <div className="flex-1 min-h-0 overflow-y-auto p-3" data-vaul-no-drag>
          <PRCIPopover pr={pr} enabled={open} />
        </div>
      </DrawerContent>
    </Drawer>
  );
}

function ChipStatusGlyph({ status }: { status: ChipStatus }) {
  switch (status) {
    case "passed":
      return <IconCircleCheckFilled className="h-3.5 w-3.5 text-green-500" aria-hidden="true" />;
    case "failed":
      return <IconCircleXFilled className="h-3.5 w-3.5 text-red-500" aria-hidden="true" />;
    case "conflict":
      return <IconAlertTriangleFilled className="h-3.5 w-3.5 text-red-500" aria-hidden="true" />;
    case "behind":
    case "blocked":
      return <IconAlertTriangleFilled className="h-3.5 w-3.5 text-yellow-500" aria-hidden="true" />;
    case "in_progress":
      // CI runs take minutes, so slow the spin to ~3s/rotation — the default
      // animate-spin (1s) feels frantic for a long-running task.
      return (
        <IconLoader2
          className="h-3.5 w-3.5 text-yellow-500 animate-spin [animation-duration:3s]"
          aria-hidden="true"
        />
      );
    default:
      return <IconPointFilled className="h-3.5 w-3.5 text-muted-foreground" aria-hidden="true" />;
  }
}
