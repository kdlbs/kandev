"use client";

import { memo, useState, type ReactNode } from "react";
import {
  IconGitPullRequest,
  IconCheck,
  IconX,
  IconClock,
  IconChevronDown,
  IconAlertTriangle,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverAnchor, PopoverContent } from "@kandev/ui/popover";
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
import { useHoverPopover } from "@/hooks/domains/github/use-hover-popover";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import {
  aggregatePRStatusColor,
  getPRStatusColor,
  hasPRChecksInProgressForDisplay,
  hasPRChecksPassedWithoutReviewWaitForDisplay,
  isPRAwaitingReview,
  isPRReadyToMerge,
  isPRWaitingOnBranchProtection,
} from "@/components/github/pr-task-icon";
import { prTaskKey } from "@/components/github/pr-detail-panel";
import { PR_CI_DESKTOP_POPOVER_SCROLL_CLASS, PRCIPopover } from "@/components/github/pr-ci-popover";
import { MultiPRCIPopover } from "@/components/github/multi-pr-ci-popover";
import { useAppStore } from "@/components/state-provider";
import type { TaskPR } from "@/lib/types/github";

const POPOVER_OPEN_DELAY_MS = 150;
const POPOVER_CLOSE_DELAY_MS = 150;

// Badge for the hard merge blockers that must beat ready/awaiting-review:
// conflicts (red) and behind-base (amber). Mirrors openMergeBlockerColor so
// the badge agrees with the pill colour. Returns null otherwise. ("blocked"
// is handled later, after awaiting-review.)
function mergeBlockerBadge(pr: TaskPR): ReactNode | null {
  if (pr.state !== "open") return null;
  if (pr.mergeable_state === "dirty") {
    return <IconAlertTriangle className="h-3 w-3 text-red-500" />;
  }
  if (pr.mergeable_state === "behind") {
    return <IconAlertTriangle className="h-3 w-3 text-yellow-500" />;
  }
  return null;
}

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
  const blockerBadge = mergeBlockerBadge(pr);
  if (blockerBadge) return blockerBadge;
  if (isPRReadyToMerge(pr)) {
    return <IconCheck className="h-3 w-3 text-emerald-400" />;
  }
  // Check awaiting-review before the plain approved check so an approved PR
  // with pending reviewers (1 of N required) doesn't read as fully approved.
  if (isPRAwaitingReview(pr)) {
    return <IconClock className="h-3 w-3 text-sky-400" />;
  }
  if (isPRWaitingOnBranchProtection(pr)) {
    return <IconClock className="h-3 w-3 text-muted-foreground" />;
  }
  if (hasPRChecksPassedWithoutReviewWaitForDisplay(pr)) {
    return <IconCheck className="h-3 w-3 text-green-500" />;
  }
  if (hasPRChecksInProgressForDisplay(pr) || pr.review_state === "pending") {
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

/**
 * Manages the hover-driven popover lifecycle. Click on the button is left
 * to the caller — desktop preserves the existing "open the PR detail panel"
 * behavior, and hover is what reveals the CI popover. On touch devices the
 * popover is suppressed entirely so the button click falls through to the
 * existing detail-panel handler.
 *
 * The hover-bridge logic (keeping the popover open while the cursor crosses
 * from the trigger onto the portalled content) lives in the shared
 * {@link useHoverPopover} hook so the chip and this button stay in sync.
 */
function usePopoverInteractions() {
  const usesTouchDrawer = useTouchDrawer();
  const hover = useHoverPopover({
    openDelayMs: POPOVER_OPEN_DELAY_MS,
    closeDelayMs: POPOVER_CLOSE_DELAY_MS,
    disabled: usesTouchDrawer,
  });
  return { usesTouchDrawer, ...hover };
}

function PRSingleButton({ pr }: { pr: TaskPR }) {
  const addPRPanel = useDockviewStore((s) => s.addPRPanel);
  const activeSessionId = useAppStore((s) => s.tasks.activeSessionId);
  const tooltip = `${pr.owner}/${pr.repo} #${pr.pr_number} — ${pr.pr_title}`;
  const {
    usesTouchDrawer,
    open,
    onOpenChange,
    onTriggerEnter,
    onTriggerLeave,
    onContentEnter,
    onContentLeave,
  } = usePopoverInteractions();
  // Background sync lives on PRStatusChip (always mounted in the chat
  // input area); the chip and this popover share prFeedbackCache so a
  // single subscription warms both.

  const button = (
    <Button
      data-testid="pr-topbar-button"
      data-pr-number={pr.pr_number}
      data-pr-state={pr.state}
      data-pr-ready-to-merge={isPRReadyToMerge(pr) ? "true" : "false"}
      size="sm"
      variant="outline"
      className="cursor-pointer gap-1.5 px-2"
      onMouseOver={onTriggerEnter}
      onMouseEnter={onTriggerEnter}
      onMouseMove={onTriggerEnter}
      onPointerOver={onTriggerEnter}
      onPointerEnter={onTriggerEnter}
      onPointerMove={onTriggerEnter}
      onMouseLeave={onTriggerLeave}
      onPointerLeave={onTriggerLeave}
      onFocus={onTriggerEnter}
      onBlur={onTriggerLeave}
      onClick={() => {
        addPRPanel(prTaskKey(pr), activeSessionId);
        onOpenChange(false);
      }}
    >
      <IconGitPullRequest className={`h-4 w-4 ${getPRStatusColor(pr)}`} />
      <span className="text-xs font-medium">#{pr.pr_number}</span>
      <PRStatusIcon pr={pr} />
    </Button>
  );

  if (usesTouchDrawer) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>{button}</TooltipTrigger>
        <TooltipContent>{tooltip}</TooltipContent>
      </Tooltip>
    );
  }

  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverAnchor asChild>{button}</PopoverAnchor>
      <PopoverContent
        data-testid="pr-topbar-popover"
        align="end"
        sideOffset={4}
        className={`w-80 ${PR_CI_DESKTOP_POPOVER_SCROLL_CLASS}`}
        onMouseEnter={onContentEnter}
        onMouseMove={onContentEnter}
        onMouseLeave={onContentLeave}
        onOpenAutoFocus={(e) => e.preventDefault()}
      >
        <PRCIPopover pr={pr} enabled={open} />
      </PopoverContent>
    </Popover>
  );
}

function PRMultiButton({ prs }: { prs: TaskPR[] }) {
  // Click still drives the dropdown (the explicit "jump to this PR's panel"
  // affordance, and the only interaction on touch). Hover adds the aggregate
  // CI popover with a tab per PR — desktop only, suppressed on touch where
  // there is no hover.
  const addPRPanel = useDockviewStore((s) => s.addPRPanel);
  const activeSessionId = useAppStore((s) => s.tasks.activeSessionId);
  const {
    usesTouchDrawer,
    open,
    onOpenChange,
    onTriggerEnter,
    onTriggerLeave,
    onContentEnter,
    onContentLeave,
  } = usePopoverInteractions();
  const [menuOpen, setMenuOpen] = useState(false);
  const aggColor = aggregatePRStatusColor(prs);

  // The trigger is the click target for the dropdown AND the hover anchor for
  // the popover. On desktop we wrap it in a PopoverAnchor so all the asChild
  // layers (Tooltip → Popover → Dropdown) collapse onto the single Button and
  // the popover positions against it. While the dropdown is open the hover
  // popover is closed and hover re-opens are suppressed, so the two overlays
  // never stack.
  const triggerButton = (
    <DropdownMenuTrigger asChild>
      <Button
        data-testid="pr-topbar-button"
        data-pr-count={prs.length}
        size="sm"
        variant="outline"
        className="cursor-pointer gap-1.5 px-2"
        onMouseOver={menuOpen ? undefined : onTriggerEnter}
        onMouseEnter={menuOpen ? undefined : onTriggerEnter}
        onMouseMove={menuOpen ? undefined : onTriggerEnter}
        onPointerOver={menuOpen ? undefined : onTriggerEnter}
        onPointerEnter={menuOpen ? undefined : onTriggerEnter}
        onPointerMove={menuOpen ? undefined : onTriggerEnter}
        onMouseLeave={onTriggerLeave}
        onPointerLeave={onTriggerLeave}
        onFocus={menuOpen ? undefined : onTriggerEnter}
        onBlur={onTriggerLeave}
      >
        <IconGitPullRequest className={`h-4 w-4 ${aggColor}`} />
        <span className="text-xs font-medium">{prs.length} PRs</span>
        <IconChevronDown className="h-3 w-3 text-muted-foreground" />
      </Button>
    </DropdownMenuTrigger>
  );

  const dropdown = (
    <DropdownMenu
      onOpenChange={(next) => {
        setMenuOpen(next);
        if (next) onOpenChange(false);
      }}
    >
      <Tooltip>
        <TooltipTrigger asChild>
          {usesTouchDrawer ? triggerButton : <PopoverAnchor asChild>{triggerButton}</PopoverAnchor>}
        </TooltipTrigger>
        <TooltipContent>{prs.length} pull requests linked to this task — open one</TooltipContent>
      </Tooltip>
      <MultiPRMenuContent prs={prs} />
    </DropdownMenu>
  );

  if (usesTouchDrawer) return dropdown;

  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      {dropdown}
      <PopoverContent
        data-testid="pr-topbar-popover"
        align="end"
        sideOffset={4}
        className={`w-96 ${PR_CI_DESKTOP_POPOVER_SCROLL_CLASS}`}
        onMouseEnter={onContentEnter}
        onMouseMove={onContentEnter}
        onMouseLeave={onContentLeave}
        onOpenAutoFocus={(e) => e.preventDefault()}
      >
        <MultiPRCIPopover
          prs={prs}
          enabled={open}
          onOpenDetailPanel={(pr) => {
            addPRPanel(prTaskKey(pr), activeSessionId);
            onOpenChange(false);
          }}
        />
      </PopoverContent>
    </Popover>
  );
}

function MultiPRMenuContent({ prs }: { prs: TaskPR[] }) {
  const addPRPanel = useDockviewStore((s) => s.addPRPanel);
  const activeSessionId = useAppStore((s) => s.tasks.activeSessionId);
  return (
    <DropdownMenuContent align="end" className="w-72">
      <DropdownMenuLabel className="text-xs">Pull requests</DropdownMenuLabel>
      <DropdownMenuSeparator />
      {prs.map((pr) => (
        <DropdownMenuItem
          key={pr.id}
          onClick={() => addPRPanel(prTaskKey(pr), activeSessionId)}
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
  );
}
