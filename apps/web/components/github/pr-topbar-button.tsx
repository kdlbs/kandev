"use client";

import { memo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronDown } from "@tabler/icons-react";
import { Popover, PopoverAnchor, PopoverContent } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSub,
  ContextMenuSubContent,
  ContextMenuSubTrigger,
  ContextMenuTrigger,
} from "@kandev/ui/context-menu";
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
import { useTaskPRUnlink } from "@/hooks/domains/github/use-task-pr-unlink";
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
  if (!triggerRef) return;
  let attempts = 0;
  const restoreFocus = () => {
    attempts += 1;
    triggerRef.current?.focus({ preventScroll: true });
    // A two-PR surface is replaced by a single-PR surface after the mutation.
    // Retry across the replacement render so the surviving trigger owns focus.
    if (attempts < 4) setTimeout(restoreFocus, 0);
  };
  setTimeout(restoreFocus, 0);
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
  const unlinkMenu = useTaskPRUnlink(activeTaskId);
  const triggerRef = useRef<HTMLButtonElement>(null);

  if (prs.length === 0) return null;
  if (prs.length === 1)
    return (
      <PRSingleButton
        pr={prs[0]}
        refreshTaskPR={refresh}
        onUnlinkPR={unlinkMenu.unlink}
        pendingIds={unlinkMenu.pendingIds}
        canUnlink={unlinkMenu.canUnlink}
        triggerRef={triggerRef}
      />
    );
  return (
    <PRMultiButton
      prs={prs}
      refreshTaskPR={refresh}
      onRemovePR={unlink}
      onUnlinkPR={unlinkMenu.unlink}
      pendingIds={unlinkMenu.pendingIds}
      canUnlink={unlinkMenu.canUnlink}
      triggerRef={triggerRef}
    />
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

type PopoverInteractionHandlers = ReturnType<typeof usePopoverInteractions>;

function buildPRSingleButtonTrigger({
  pr,
  statusText,
  triggerRef,
  contextMenuOpen,
  onClick,
  onTriggerEnter,
  onTriggerLeave,
}: {
  pr: TaskPR;
  statusText: string;
  triggerRef?: TriggerRef;
  contextMenuOpen: boolean;
  onClick: () => void;
  onTriggerEnter: PopoverInteractionHandlers["onTriggerEnter"];
  onTriggerLeave: PopoverInteractionHandlers["onTriggerLeave"];
}) {
  return (
    <ChangeRequestTopbarButton
      ref={triggerRef}
      data-testid="pr-topbar-button"
      data-pr-number={pr.pr_number}
      data-pr-state={pr.state}
      data-pr-ready-to-merge={isPRReadyToMerge(pr) ? "true" : "false"}
      aria-label={statusText}
      onMouseOver={contextMenuOpen ? undefined : onTriggerEnter}
      onMouseEnter={contextMenuOpen ? undefined : onTriggerEnter}
      onMouseMove={contextMenuOpen ? undefined : onTriggerEnter}
      onPointerOver={contextMenuOpen ? undefined : onTriggerEnter}
      onPointerEnter={contextMenuOpen ? undefined : onTriggerEnter}
      onPointerMove={contextMenuOpen ? undefined : onTriggerEnter}
      onMouseLeave={onTriggerLeave}
      onPointerLeave={onTriggerLeave}
      onFocus={contextMenuOpen ? undefined : onTriggerEnter}
      onBlur={onTriggerLeave}
      onClick={onClick}
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
}

function PRSingleButton({
  pr,
  refreshTaskPR,
  onUnlinkPR,
  pendingIds,
  canUnlink,
  triggerRef,
}: {
  pr: TaskPR;
  refreshTaskPR: () => void | Promise<void>;
  onUnlinkPR: (associationId: string) => Promise<boolean>;
  pendingIds: ReadonlySet<string>;
  canUnlink: boolean;
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
  const [contextMenuOpen, setContextMenuOpen] = useState(false);
  // Background sync lives on PRStatusChip (always mounted in the chat
  // input area); the chip and this popover share prFeedbackCache so a
  // single subscription warms both.

  const button = buildPRSingleButtonTrigger({
    pr,
    statusText,
    triggerRef,
    contextMenuOpen,
    onClick: () => {
      addPRPanel(prTaskKey(pr));
      onOpenChange(false);
    },
    onTriggerEnter,
    onTriggerLeave,
  });

  if (usesTouchDrawer) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>{button}</TooltipTrigger>
        <TooltipContent>{tooltip}</TooltipContent>
      </Tooltip>
    );
  }

  return (
    <ContextMenu
      onOpenChange={(next) => {
        setContextMenuOpen(next);
        if (next) onOpenChange(false);
      }}
    >
      <Popover open={open} onOpenChange={onOpenChange}>
        <PopoverAnchor asChild>
          <ContextMenuTrigger asChild>{button}</ContextMenuTrigger>
        </PopoverAnchor>
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
      <PRTopbarUnlinkContextContent
        prs={[pr]}
        onUnlinkPR={onUnlinkPR}
        pendingIds={pendingIds}
        canUnlink={canUnlink}
      />
    </ContextMenu>
  );
}

function buildPRMultiButtonTrigger({
  prs,
  colorClassName,
  label,
  triggerRef,
  menuOpen,
  contextMenuOpen,
  onTriggerEnter,
  onTriggerLeave,
}: {
  prs: TaskPR[];
  colorClassName: string;
  label: string;
  triggerRef?: TriggerRef;
  menuOpen: boolean;
  contextMenuOpen: boolean;
  onTriggerEnter: PopoverInteractionHandlers["onTriggerEnter"];
  onTriggerLeave: PopoverInteractionHandlers["onTriggerLeave"];
}) {
  const hoverSuppressed = menuOpen || contextMenuOpen;
  return (
    <DropdownMenuTrigger asChild>
      <ChangeRequestTopbarButton
        ref={triggerRef}
        data-testid="pr-topbar-button"
        data-pr-count={prs.length}
        aria-label={label}
        onMouseOver={hoverSuppressed ? undefined : onTriggerEnter}
        onMouseEnter={hoverSuppressed ? undefined : onTriggerEnter}
        onMouseMove={hoverSuppressed ? undefined : onTriggerEnter}
        onPointerOver={hoverSuppressed ? undefined : onTriggerEnter}
        onPointerEnter={hoverSuppressed ? undefined : onTriggerEnter}
        onPointerMove={hoverSuppressed ? undefined : onTriggerEnter}
        onMouseLeave={onTriggerLeave}
        onPointerLeave={onTriggerLeave}
        onFocus={hoverSuppressed ? undefined : onTriggerEnter}
        onBlur={onTriggerLeave}
      >
        <PRMultiButtonContent prs={prs} colorClassName={colorClassName} />
      </ChangeRequestTopbarButton>
    </DropdownMenuTrigger>
  );
}

function buildPRMultiTriggerSurface(trigger: React.ReactElement, usesTouchDrawer: boolean) {
  if (usesTouchDrawer) return trigger;
  return (
    <PopoverAnchor asChild>
      <ContextMenuTrigger asChild>{trigger}</ContextMenuTrigger>
    </PopoverAnchor>
  );
}

function PRMultiDesktopSurface({
  dropdown,
  prs,
  refreshTaskPR,
  onRemovePR,
  onUnlinkPR,
  pendingIds,
  canUnlink,
  triggerRef,
  interactions,
  setContextMenuOpen,
  onOpenDetailPanel,
}: {
  dropdown: React.ReactElement;
  prs: TaskPR[];
  refreshTaskPR: () => void | Promise<void>;
  onRemovePR: (associationId: string) => Promise<void>;
  onUnlinkPR: (associationId: string) => Promise<boolean>;
  pendingIds: ReadonlySet<string>;
  canUnlink: boolean;
  triggerRef?: TriggerRef;
  interactions: PopoverInteractionHandlers;
  setContextMenuOpen: (open: boolean) => void;
  onOpenDetailPanel: (pr: TaskPR) => void;
}) {
  const onContextMenuOpenChange = (next: boolean) => {
    setContextMenuOpen(next);
    if (next) interactions.onOpenChange(false);
  };
  return (
    <ContextMenu onOpenChange={onContextMenuOpenChange}>
      <Popover open={interactions.open} onOpenChange={interactions.onOpenChange}>
        {dropdown}
        <PopoverContent
          data-testid="pr-topbar-popover"
          align="end"
          sideOffset={4}
          className={`w-96 ${PR_CI_DESKTOP_POPOVER_SCROLL_CLASS}`}
          onMouseEnter={interactions.onContentEnter}
          onMouseMove={interactions.onContentEnter}
          onMouseLeave={interactions.onContentLeave}
          onOpenAutoFocus={(event) => event.preventDefault()}
        >
          <MultiPRCIPopover
            prs={prs}
            enabled={interactions.open}
            refreshTaskPR={refreshTaskPR}
            onRemovePR={(pr) => onRemovePR(pr.id)}
            onCollapseFocus={() => focusAfterCollapse(triggerRef)}
            onOpenDetailPanel={onOpenDetailPanel}
          />
        </PopoverContent>
      </Popover>
      <PRTopbarUnlinkContextContent
        prs={prs}
        onUnlinkPR={onUnlinkPR}
        pendingIds={pendingIds}
        canUnlink={canUnlink}
      />
    </ContextMenu>
  );
}

function PRMultiButton({
  prs,
  refreshTaskPR,
  onRemovePR,
  onUnlinkPR,
  pendingIds,
  canUnlink,
  triggerRef,
}: {
  prs: TaskPR[];
  refreshTaskPR: () => void | Promise<void>;
  onRemovePR: (associationId: string) => Promise<void>;
  onUnlinkPR: (associationId: string) => Promise<boolean>;
  pendingIds: ReadonlySet<string>;
  canUnlink: boolean;
  triggerRef?: TriggerRef;
}) {
  const { t } = useTranslation();
  const addPRPanel = useDockviewStore((s) => s.addPRPanel);
  const interactions = usePopoverInteractions();
  const [menuOpen, setMenuOpen] = useState(false);
  const [contextMenuOpen, setContextMenuOpen] = useState(false);
  const trigger = buildPRMultiButtonTrigger({
    prs,
    colorClassName: aggregatePRStatusColor(prs),
    label: multiPRAccessibleStatus(prs, t),
    triggerRef,
    menuOpen,
    contextMenuOpen,
    onTriggerEnter: interactions.onTriggerEnter,
    onTriggerLeave: interactions.onTriggerLeave,
  });
  const dropdown = (
    <DropdownMenu
      onOpenChange={(next) => {
        setMenuOpen(next);
        if (next) interactions.onOpenChange(false);
      }}
    >
      <Tooltip>
        <TooltipTrigger asChild>
          {buildPRMultiTriggerSurface(trigger, interactions.usesTouchDrawer)}
        </TooltipTrigger>
        <TooltipContent>
          {t("github:pullRequestsLinkedToTask", { count: prs.length })}
        </TooltipContent>
      </Tooltip>
      <MultiPRMenuContent prs={prs} />
    </DropdownMenu>
  );

  if (interactions.usesTouchDrawer) return dropdown;
  return (
    <PRMultiDesktopSurface
      dropdown={dropdown}
      prs={prs}
      refreshTaskPR={refreshTaskPR}
      onRemovePR={onRemovePR}
      onUnlinkPR={onUnlinkPR}
      pendingIds={pendingIds}
      canUnlink={canUnlink}
      triggerRef={triggerRef}
      interactions={interactions}
      setContextMenuOpen={setContextMenuOpen}
      onOpenDetailPanel={(pr) => {
        addPRPanel(prTaskKey(pr));
        interactions.onOpenChange(false);
      }}
    />
  );
}

function PRTopbarUnlinkContextContent({
  prs,
  onUnlinkPR,
  pendingIds,
  canUnlink,
}: {
  prs: TaskPR[];
  onUnlinkPR: (associationId: string) => Promise<boolean>;
  pendingIds: ReadonlySet<string>;
  canUnlink: boolean;
}) {
  const { t } = useTranslation();
  return (
    <ContextMenuContent>
      <ContextMenuSub>
        <ContextMenuSubTrigger>{t("common:edit")}</ContextMenuSubTrigger>
        <ContextMenuSubContent className="w-64">
          {prs.map((pr) => {
            const repo = pr.owner ? `${pr.owner}/${pr.repo}` : pr.repo;
            const label = t("github:removeFromTask", { repo, prnumber: pr.pr_number });
            return (
              <ContextMenuItem
                key={pr.id}
                data-testid={`pr-topbar-unlink-${prIdentitySlug(pr)}`}
                aria-label={label}
                disabled={!canUnlink || pendingIds.has(pr.id)}
                className="cursor-pointer"
                onSelect={() => void onUnlinkPR(pr.id)}
              >
                {label}
              </ContextMenuItem>
            );
          })}
        </ContextMenuSubContent>
      </ContextMenuSub>
    </ContextMenuContent>
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
