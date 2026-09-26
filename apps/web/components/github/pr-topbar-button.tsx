"use client";

import { memo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronDown } from "@tabler/icons-react";
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
  hasAnyPRMergeConflict,
  hasPRMergeConflict,
  getPRStatusAccessibleLabels,
  isPRReadyToMerge,
} from "@/components/github/pr-task-icon";
import { PRStatusGlyph } from "@/components/github/pr-status-glyph";
import { prIdentitySlug, prTaskKey } from "@/components/github/pr-utils";
import { PR_CI_DESKTOP_POPOVER_SCROLL_CLASS, PRCIPopover } from "@/components/github/pr-ci-popover";
import { MultiPRCIPopover } from "@/components/github/multi-pr-ci-popover";
import { useAppStore } from "@/components/state-provider";
import {
  ChangeRequestTopbarButton,
  ChangeRequestTopbarContent,
} from "@/components/integrations/change-request-status-chrome";
import type { TaskPR } from "@/lib/types/github";

const POPOVER_OPEN_DELAY_MS = 150;
const POPOVER_CLOSE_DELAY_MS = 150;
type TriggerRef = { current: HTMLButtonElement | null };

function prTopbarAccessibleStatus(pr: TaskPR, t: ReturnType<typeof useTranslation>["t"]): string {
  const parts = [
    t("github:pullRequestStatus", { number: pr.pr_number }),
    ...getPRStatusAccessibleLabels(pr, t),
  ];
  if (hasPRMergeConflict(pr)) parts.push(t("github:conflicts"));
  return parts.join(", ");
}

function multiPRAccessibleStatus(prs: TaskPR[], t: ReturnType<typeof useTranslation>["t"]): string {
  const parts = [
    `${prs.length} ${t("github:prs")}`,
    ...new Set(prs.flatMap((pr) => getPRStatusAccessibleLabels(pr, t))),
  ];
  if (hasAnyPRMergeConflict(prs)) parts.push(t("github:conflicts"));
  return parts.join(", ");
}

function focusAfterCollapse(triggerRef?: TriggerRef) {
  if (triggerRef) setTimeout(() => triggerRef.current?.focus(), 0);
}

function PRMultiButtonContent({ prs, colorClassName }: { prs: TaskPR[]; colorClassName: string }) {
  const { t } = useTranslation();
  return (
    <ChangeRequestTopbarContent
      label={`${prs.length} ${t("github:prs")}`}
      colorClassName={colorClassName}
      leadingIcon={
        <PRStatusGlyph
          size="topbar"
          colorClassName={colorClassName}
          hasMergeConflicts={hasAnyPRMergeConflict(prs)}
        />
      }
      dropdown={<IconChevronDown className="h-3 w-3 text-muted-foreground" />}
    />
  );
}

export const PRTopbarButton = memo(function PRTopbarButton() {
  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  // useTaskPR fetches if not in store and returns the full per-task list so
  // multi-repo tasks can surface every PR (one button for single-repo, a
  // dropdown summary for 2+ so the topbar doesn't blow up horizontally).
  const { prs, refresh, unlink } = useTaskPR(activeTaskId);
  const triggerRef = useRef<HTMLButtonElement>(null);

  if (prs.length === 0) return null;
  if (prs.length === 1)
    return <PRSingleButton pr={prs[0]} refreshTaskPR={refresh} triggerRef={triggerRef} />;
  return (
    <PRMultiButton prs={prs} refreshTaskPR={refresh} onRemovePR={unlink} triggerRef={triggerRef} />
  );
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

function PRSingleButton({
  pr,
  refreshTaskPR,
  triggerRef,
}: {
  pr: TaskPR;
  refreshTaskPR: () => void | Promise<void>;
  triggerRef?: TriggerRef;
}) {
  const { t } = useTranslation();
  const addPRPanel = useDockviewStore((s) => s.addPRPanel);
  const tooltip = `${pr.owner}/${pr.repo} #${pr.pr_number} - ${pr.pr_title}`;
  const statusText = prTopbarAccessibleStatus(pr, t);
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
    <ChangeRequestTopbarButton
      ref={triggerRef}
      data-testid="pr-topbar-button"
      data-pr-number={pr.pr_number}
      data-pr-state={pr.state}
      data-pr-ready-to-merge={isPRReadyToMerge(pr) ? "true" : "false"}
      aria-label={statusText}
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
        addPRPanel(prTaskKey(pr));
        onOpenChange(false);
      }}
    >
      <ChangeRequestTopbarContent
        label={`#${pr.pr_number}`}
        colorClassName={getPRStatusColor(pr)}
        leadingIcon={
          <PRStatusGlyph
            size="topbar"
            colorClassName={getPRStatusColor(pr)}
            hasMergeConflicts={hasPRMergeConflict(pr)}
          />
        }
      />
    </ChangeRequestTopbarButton>
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
        <PRCIPopover pr={pr} enabled={open} refreshTaskPR={refreshTaskPR} />
      </PopoverContent>
    </Popover>
  );
}

function PRMultiButton({
  prs,
  refreshTaskPR,
  onRemovePR,
  triggerRef,
}: {
  prs: TaskPR[];
  refreshTaskPR: () => void | Promise<void>;
  onRemovePR: (associationId: string) => Promise<void>;
  triggerRef?: TriggerRef;
}) {
  const { t } = useTranslation();
  // Click opens the dropdown; desktop hover opens aggregate CI details.
  const addPRPanel = useDockviewStore((s) => s.addPRPanel);
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

  // The trigger is the dropdown target and hover anchor. On desktop the asChild
  // layers (Tooltip → Popover → Dropdown) collapse onto the single Button and
  // the popover positions against it. While the dropdown is open the hover
  // popover is closed and hover re-opens are suppressed, so the two overlays
  // never stack.
  const triggerButton = (
    <DropdownMenuTrigger asChild>
      <ChangeRequestTopbarButton
        ref={triggerRef}
        data-testid="pr-topbar-button"
        data-pr-count={prs.length}
        aria-label={multiPRAccessibleStatus(prs, t)}
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
        <PRMultiButtonContent prs={prs} colorClassName={aggColor} />
      </ChangeRequestTopbarButton>
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
        <TooltipContent>
          {t("github:pullRequestsLinkedToTask", { count: prs.length })}
        </TooltipContent>
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
          refreshTaskPR={refreshTaskPR}
          onRemovePR={(pr) => onRemovePR(pr.id)}
          onCollapseFocus={() => focusAfterCollapse(triggerRef)}
          onOpenDetailPanel={(pr) => {
            addPRPanel(prTaskKey(pr));
            onOpenChange(false);
          }}
        />
      </PopoverContent>
    </Popover>
  );
}

function MultiPRMenuContent({ prs }: { prs: TaskPR[] }) {
  const { t } = useTranslation();
  const addPRPanel = useDockviewStore((s) => s.addPRPanel);
  return (
    <DropdownMenuContent align="end" className="w-72">
      <DropdownMenuLabel className="text-xs">{t("github:pullRequests")}</DropdownMenuLabel>
      <DropdownMenuSeparator />
      {prs.map((pr) => (
        <DropdownMenuItem
          key={pr.id}
          onClick={() => addPRPanel(prTaskKey(pr))}
          className="cursor-pointer gap-2"
          data-testid={`pr-topbar-menu-item-${prIdentitySlug(pr)}`}
          aria-label={`${pr.repo}, ${prTopbarAccessibleStatus(pr, t)}`}
        >
          <PRStatusGlyph
            size="topbar"
            colorClassName={getPRStatusColor(pr)}
            hasMergeConflicts={hasPRMergeConflict(pr)}
          />
          <div className="flex flex-col min-w-0 flex-1">
            <span className="text-xs font-medium">
              {pr.repo} #{pr.pr_number}
            </span>
            <span className="text-[11px] text-muted-foreground truncate">{pr.pr_title}</span>
          </div>
        </DropdownMenuItem>
      ))}
    </DropdownMenuContent>
  );
}
