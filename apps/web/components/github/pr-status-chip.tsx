"use client";

import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  IconCircleCheckFilled,
  IconCircleXFilled,
  IconChecklist,
  IconClock,
  IconLoader2,
  IconPointFilled,
  IconAlertTriangleFilled,
  IconShield,
} from "@tabler/icons-react";
import { Drawer } from "@kandev/ui/drawer";
import { Popover, PopoverAnchor, PopoverContent } from "@kandev/ui/popover";
import { useTaskPR } from "@/hooks/domains/github/use-task-pr";
import { useHoverPopover } from "@/hooks/domains/github/use-hover-popover";
import { usePRFeedbackBackgroundSync } from "@/hooks/domains/github/use-pr-ci-popover";
import { PR_CI_DESKTOP_POPOVER_SCROLL_CLASS, PRCIPopover } from "@/components/github/pr-ci-popover";
import { useTaskCIAutomationOptions } from "@/hooks/domains/github/use-task-ci-options";
import { MultiPRCIPopover } from "@/components/github/multi-pr-ci-popover";
import { useAppStore } from "@/components/state-provider";
import {
  hasPRChecksInProgressForDisplay,
  hasPRChecksPassedForDisplay,
  isPRDraft,
  isPRAwaitingReview,
  isPRReadyToMerge,
  isPRWaitingOnBranchProtection,
  pickDefaultPR,
} from "@/components/github/pr-task-icon";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import {
  ChangeRequestStatusChip,
  ChangeRequestStatusChipHoverArea,
  ChangeRequestStatusDrawerContent,
  useChangeRequestStatusChipTriggerGuard,
} from "@/components/integrations/change-request-status-chrome";
import { autoFixRoundForState, findCIAutomationStateForPR } from "@/lib/github/ci-automation";
import type { AutoFixRoundInfo } from "@/lib/github/ci-automation";
import type { TaskCIAutomationOptions, TaskPR } from "@/lib/types/github";

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
  | "draft"
  | "waiting"
  | "in_progress"
  | "neutral";
type AutomationFlags = {
  autoFix: boolean;
  autoMerge: boolean;
  autoFixRound: AutoFixRoundInfo | null;
};
type TriggerRef = { current: HTMLButtonElement | null };
type SingleChipProps = {
  pr: TaskPR;
  automation: AutomationFlags;
  refreshTaskPR: () => void | Promise<void>;
  triggerRef?: TriggerRef;
};
type MultiChipProps = {
  prs: TaskPR[];
  statusPrs?: TaskPR[];
  automation: AutomationFlags;
  refreshTaskPR: () => void | Promise<void>;
  onRemovePR?: (pr: TaskPR) => Promise<void>;
  triggerRef?: TriggerRef;
};

function focusAfterCollapse(triggerRef?: TriggerRef) {
  if (triggerRef) setTimeout(() => triggerRef.current?.focus(), 0);
}

function chipStatus(pr: TaskPR): ChipStatus {
  if (pr.review_state === "changes_requested" || pr.checks_state === "failure") return "failed";
  // Merge conflicts / behind-base block the merge even when CI is green — the
  // chip must never read as a passed check in that case. Mirrors
  // getPRStatusColor + PRStatusIcon (dirty = red, behind = amber).
  if (pr.mergeable_state === "dirty") return "conflict";
  if (pr.mergeable_state === "behind") return "behind";
  if (isPRDraft(pr)) return "draft";
  // Pending checks / pending review must beat checks_state === "success" so a
  // PR with all checks green but reviewers still outstanding renders as
  // in-progress, not passed. Without this order, the chip flips to green the
  // moment CI finishes and ignores the human gate. isPRAwaitingReview also
  // covers approved PRs where branch protection requires more reviewers.
  if (hasPRChecksInProgressForDisplay(pr) || pr.review_state === "pending") {
    return "in_progress";
  }
  // Mirror getPRStatusColor priority: ready-to-merge beats awaiting-review so
  // the chip and icon never disagree on a (theoretical) clean+approved+pending PR.
  if (isPRAwaitingReview(pr) && !isPRReadyToMerge(pr)) return "in_progress";
  if (isPRWaitingOnBranchProtection(pr)) return "waiting";
  if (pr.mergeable_state === "blocked") return "blocked";
  if (hasPRChecksPassedForDisplay(pr)) return "passed";
  return "neutral";
}

// Higher = more attention-worthy. Drives the aggregate glyph when a task has
// multiple open PRs — one failing/conflicting PR colours the whole chip.
const CHIP_STATUS_RANK: Record<ChipStatus, number> = {
  failed: 6,
  conflict: 5,
  blocked: 4,
  behind: 3,
  draft: 0.5,
  in_progress: 2,
  waiting: 1.5,
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

/**
 * Radix HoverCard treats the trigger as outside the content's bounding box, so
 * a click on the chip would auto-close the popover. This guard filters out
 * trigger clicks so clicking the chip is a no-op while the popover stays open
 * via hover. Returns the trigger ref plus a memoised handler that reads the ref
 * lazily (inside the callback, never during render).
 */
// Hover-bridge lifecycle for the chip's desktop popover. Delegates to the
// shared hook so the chip and the top-bar PR button keep identical
// trigger->content bridge behavior (the popover must survive the cursor
// crossing the sideOffset gap). The chip's mobile surface is a Drawer, so no
// mobile guard is needed here.
function useChipPopoverInteractions() {
  return useHoverPopover({
    openDelayMs: HOVER_OPEN_DELAY_MS,
    closeDelayMs: HOVER_CLOSE_DELAY_MS,
  });
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
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const { prs, refresh, unlink } = useTaskPR(taskId);
  const { options: automationOptions } = useTaskCIAutomationOptions(taskId);
  const triggerRef = useRef<HTMLButtonElement>(null);
  // Defensive Array.isArray: a partial hydration can briefly seed the store
  // with a non-array value (same guard as PRTaskIcon).
  // Only open PRs are worth a CI chip — terminal PRs (merged/closed) are
  // already conveyed by the chat-input banner. With multiple PRs the chip
  // stays visible as long as at least one is still open.
  const allPRs = Array.isArray(prs) ? prs : [];
  const openPRs = allPRs.filter((p) => p.state !== "merged" && p.state !== "closed");
  // Subscribe at the chip level so the cache warms even when the top-bar PR
  // button isn't mounted (e.g. small viewport that hides it). Warm the PR the
  // popover will actually open first (worst-status via pickDefaultPR — for a
  // single PR that's just the PR itself); the remaining PRs in a multi-PR
  // task warm when the popover opens.
  usePRFeedbackBackgroundSync(workspaceId, pickDefaultPR(openPRs));
  if (allPRs.length === 0 || (allPRs.length === 1 && openPRs.length === 0)) return null;
  if (allPRs.length === 1)
    return (
      <PRStatusChipInner
        pr={openPRs[0]}
        automation={automationForPR(automationOptions, openPRs[0])}
        refreshTaskPR={refresh}
        triggerRef={triggerRef}
      />
    );
  return (
    <PRStatusChipMultiInner
      prs={allPRs}
      statusPrs={openPRs}
      automation={automationForPRs(automationOptions, openPRs)}
      refreshTaskPR={refresh}
      onRemovePR={(pr) => unlink(pr.id)}
      triggerRef={triggerRef}
    />
  );
}

function automationForPR(
  options: TaskCIAutomationOptions | null | undefined,
  pr: TaskPR,
): AutomationFlags {
  return {
    autoFix: Boolean(options?.auto_fix_enabled),
    autoMerge: Boolean(options?.auto_merge_enabled),
    autoFixRound: options?.auto_fix_enabled
      ? autoFixRoundForState(
          findCIAutomationStateForPR(options.pr_states, pr),
          options.auto_fix_max_rounds,
        )
      : null,
  };
}

function automationForPRs(
  options: TaskCIAutomationOptions | null | undefined,
  prs: TaskPR[],
): AutomationFlags {
  const roundInfos = options?.auto_fix_enabled
    ? prs.map((pr) =>
        autoFixRoundForState(
          findCIAutomationStateForPR(options.pr_states, pr),
          options.auto_fix_max_rounds,
        ),
      )
    : [];
  return {
    autoFix: Boolean(options?.auto_fix_enabled),
    autoMerge: Boolean(options?.auto_merge_enabled),
    autoFixRound: pickAttentionRound(roundInfos),
  };
}

function pickAttentionRound(roundInfos: AutoFixRoundInfo[]): AutoFixRoundInfo | null {
  if (roundInfos.length === 0) return null;
  return roundInfos.reduce((best, next) => {
    if (next.exhausted && !best.exhausted) return next;
    if (next.exhausted === best.exhausted && next.current > best.current) return next;
    return best;
  });
}

type ChipButtonAttrs = {
  "data-testid": "pr-status-chip";
  "data-pr-number": number;
  "data-pr-state": string;
  "data-status": ChipStatus;
  "data-pr-ready-to-merge": "true" | "false";
  "aria-label": string;
};

function automationAriaSuffix(automation: AutomationFlags): string {
  const flags = [
    automation.autoFix
      ? `auto-fix enabled${automation.autoFixRound ? ` ${automation.autoFixRound.current} of ${automation.autoFixRound.max} rounds used` : ""}`
      : null,
    automation.autoMerge ? "auto-merge enabled" : null,
  ].filter(Boolean);
  return flags.length > 0 ? `, ${flags.join(", ")}` : "";
}

function chipButtonAttrs(
  pr: TaskPR,
  status: ChipStatus,
  automation: AutomationFlags,
): ChipButtonAttrs {
  return {
    "data-testid": "pr-status-chip",
    "data-pr-number": pr.pr_number,
    "data-pr-state": pr.state,
    "data-status": status,
    "data-pr-ready-to-merge": isPRReadyToMerge(pr) ? "true" : "false",
    "aria-label": `Pull request #${pr.pr_number} CI status${automationAriaSuffix(automation)}`,
  };
}

function AutomationFlagBadges({ automation }: { automation: AutomationFlags }) {
  const { t } = useTranslation();
  if (!automation.autoFix && !automation.autoMerge) return null;
  const autoFixRound = automation.autoFixRound;
  return (
    <>
      {automation.autoFix && autoFixRound && (
        <span
          data-testid="pr-status-auto-fix-chip"
          data-auto-fix-round={`${autoFixRound.current}/${autoFixRound.max}`}
          data-auto-fix-exhausted={autoFixRound.exhausted ? "true" : "false"}
          className={`rounded-sm px-1 py-0.5 text-[9px] font-medium leading-none ${
            autoFixRound.exhausted
              ? "bg-yellow-500/15 text-yellow-500"
              : "bg-emerald-500/15 text-emerald-500"
          }`}
        >
          {t("github:autoFix")} {autoFixRound.current}/{autoFixRound.max}
        </span>
      )}
      {automation.autoMerge && (
        <span
          data-testid="pr-status-auto-merge-chip"
          className="rounded-sm bg-sky-500/15 px-1 py-0.5 text-[9px] font-medium leading-none text-sky-500"
        >
          {t("github:autoMerge")}
        </span>
      )}
    </>
  );
}

function PRStatusChipInner(props: SingleChipProps) {
  const usesMobileDrawer = useTouchDrawer();
  if (usesMobileDrawer) return <PRStatusChipDrawer {...props} />;
  return <PRStatusChipHoverCard {...props} />;
}

function PRStatusChipHoverCard({ pr, automation, refreshTaskPR, triggerRef }: SingleChipProps) {
  const status = chipStatus(pr);
  const { ref, onPointerDownOutside } = useChangeRequestStatusChipTriggerGuard(triggerRef);
  const { open, onOpenChange, onTriggerEnter, onTriggerLeave, onContentEnter, onContentLeave } =
    useChipPopoverInteractions();
  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <ChangeRequestStatusChipHoverArea handlers={{ onTriggerEnter, onTriggerLeave }}>
        <PopoverAnchor asChild>
          <ChangeRequestStatusChip ref={ref} {...chipButtonAttrs(pr, status, automation)}>
            <IconChecklist className="h-3.5 w-3.5 text-muted-foreground" aria-hidden="true" />
            <ChipStatusGlyph status={status} />
            <AutomationFlagBadges automation={automation} />
          </ChangeRequestStatusChip>
        </PopoverAnchor>
      </ChangeRequestStatusChipHoverArea>
      <PopoverContent
        side="top"
        align="start"
        sideOffset={8}
        className={`w-80 p-2.5 ${PR_CI_DESKTOP_POPOVER_SCROLL_CLASS}`}
        onMouseEnter={onContentEnter}
        onMouseMove={onContentEnter}
        onMouseLeave={onContentLeave}
        onPointerDownOutside={onPointerDownOutside}
        onOpenAutoFocus={(e) => e.preventDefault()}
      >
        <PRCIPopover pr={pr} enabled={open} refreshTaskPR={refreshTaskPR} />
      </PopoverContent>
    </Popover>
  );
}

function PRStatusChipMultiInner(props: MultiChipProps) {
  const usesMobileDrawer = useTouchDrawer();
  if (usesMobileDrawer) return <PRStatusChipMultiDrawer {...props} />;
  return <PRStatusChipMultiHoverCard {...props} />;
}

type MultiChipButtonAttrs = {
  "data-testid": "pr-status-chip";
  "data-pr-count": number;
  "data-status": ChipStatus;
  "aria-label": string;
};

function multiChipButtonAttrs(
  prs: TaskPR[],
  status: ChipStatus,
  automation: AutomationFlags,
): MultiChipButtonAttrs {
  return {
    "data-testid": "pr-status-chip",
    "data-pr-count": prs.length,
    "data-status": status,
    "aria-label": `${prs.length} pull requests CI status${automationAriaSuffix(automation)}`,
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
  statusPrs,
  automation,
  refreshTaskPR,
  onRemovePR,
  triggerRef,
}: MultiChipProps) {
  const status = aggregateChipStatus(statusPrs ?? prs);
  const { ref, onPointerDownOutside } = useChangeRequestStatusChipTriggerGuard(triggerRef);
  const { open, onOpenChange, onTriggerEnter, onTriggerLeave, onContentEnter, onContentLeave } =
    useChipPopoverInteractions();
  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <ChangeRequestStatusChipHoverArea handlers={{ onTriggerEnter, onTriggerLeave }}>
        <PopoverAnchor asChild>
          <ChangeRequestStatusChip ref={ref} {...multiChipButtonAttrs(prs, status, automation)}>
            <MultiChipGlyph prs={prs} status={status} automation={automation} />
          </ChangeRequestStatusChip>
        </PopoverAnchor>
      </ChangeRequestStatusChipHoverArea>
      <PopoverContent
        side="top"
        align="start"
        sideOffset={8}
        className={`w-96 p-2.5 ${PR_CI_DESKTOP_POPOVER_SCROLL_CLASS}`}
        onMouseEnter={onContentEnter}
        onMouseMove={onContentEnter}
        onMouseLeave={onContentLeave}
        onPointerDownOutside={onPointerDownOutside}
        onOpenAutoFocus={(e) => e.preventDefault()}
      >
        <MultiPRCIPopover
          prs={prs}
          enabled={open}
          refreshTaskPR={refreshTaskPR}
          onRemovePR={onRemovePR}
          onCollapseFocus={() => focusAfterCollapse(triggerRef)}
        />
      </PopoverContent>
    </Popover>
  );
}

function PRStatusChipMultiDrawer({
  prs,
  statusPrs,
  automation,
  refreshTaskPR,
  onRemovePR,
  triggerRef,
}: MultiChipProps) {
  const { t } = useTranslation();
  const status = aggregateChipStatus(statusPrs ?? prs);
  const [open, setOpen] = useState(false);
  return (
    <Drawer open={open} onOpenChange={setOpen}>
      <ChangeRequestStatusChip
        ref={triggerRef}
        aria-haspopup="dialog"
        aria-expanded={open}
        onClick={() => setOpen(true)}
        {...multiChipButtonAttrs(prs, status, automation)}
      >
        <MultiChipGlyph prs={prs} status={status} automation={automation} />
      </ChangeRequestStatusChip>
      <ChangeRequestStatusDrawerContent
        testId="pr-status-chip-drawer"
        closeTestId="pr-status-chip-drawer-close"
        title={t("github:pullRequestCount", { count: prs.length })}
        description={t("github:pullRequestCiStatusReviewsAnd")}
        closeLabel={t("github:closePrStatus")}
      >
        <MultiPRCIPopover
          prs={prs}
          enabled={open}
          refreshTaskPR={refreshTaskPR}
          onRemovePR={onRemovePR}
          onCollapseFocus={() => focusAfterCollapse(triggerRef)}
        />
      </ChangeRequestStatusDrawerContent>
    </Drawer>
  );
}

function PRStatusChipDrawer({ pr, automation, refreshTaskPR, triggerRef }: SingleChipProps) {
  const { t } = useTranslation();
  const status = chipStatus(pr);
  const [open, setOpen] = useState(false);
  return (
    <Drawer open={open} onOpenChange={setOpen}>
      <ChangeRequestStatusChip
        ref={triggerRef}
        aria-haspopup="dialog"
        aria-expanded={open}
        onClick={() => setOpen(true)}
        {...chipButtonAttrs(pr, status, automation)}
      >
        <IconChecklist className="h-3.5 w-3.5 text-muted-foreground" aria-hidden="true" />
        <ChipStatusGlyph status={status} />
        <AutomationFlagBadges automation={automation} />
      </ChangeRequestStatusChip>
      <ChangeRequestStatusDrawerContent
        testId="pr-status-chip-drawer"
        closeTestId="pr-status-chip-drawer-close"
        title={`${t("github:pr")}${pr.pr_number}`}
        description={t("github:pullRequestCiStatusReviewsAnd")}
        closeLabel={t("github:closePrStatus")}
      >
        <PRCIPopover pr={pr} enabled={open} refreshTaskPR={refreshTaskPR} />
      </ChangeRequestStatusDrawerContent>
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
      return <IconAlertTriangleFilled className="h-3.5 w-3.5 text-yellow-500" aria-hidden="true" />;
    case "draft":
      return <IconPointFilled className="h-3.5 w-3.5 text-muted-foreground" aria-hidden="true" />;
    case "blocked":
      return (
        <IconShield
          data-testid="pr-status-glyph-blocked"
          className="h-3.5 w-3.5 text-yellow-500"
          aria-hidden="true"
        />
      );
    case "waiting":
      return <IconClock className="h-3.5 w-3.5 text-muted-foreground" aria-hidden="true" />;
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
