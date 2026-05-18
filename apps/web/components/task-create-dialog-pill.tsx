"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { cn } from "@/lib/utils";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { Badge } from "@kandev/ui/badge";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@kandev/ui/command";
import type { Branch } from "@/lib/types/http";
import { BranchRefreshButton } from "@/components/branch-refresh-button";

export type PillOption = {
  value: string;
  label: string;
  keywords?: string[];
  renderLabel?: () => React.ReactNode;
};

type PillProps = {
  icon: React.ReactNode;
  value: string;
  placeholder: string;
  options: PillOption[];
  onSelect: (value: string) => void;
  disabled?: boolean;
  /** When provided alongside `disabled`, surfaces a tooltip explaining why. */
  disabledReason?: string;
  searchPlaceholder: string;
  emptyMessage: string;
  testId?: string;
  /** Optional refresh action rendered next to the search input. */
  onRefresh?: () => void;
  /** Show the refresh icon as spinning + disabled while a refresh is in flight. */
  refreshing?: boolean;
  /**
   * Render without its own border/bg so the pill blends into a wrapping
   * grouped container (used by RepoChip to draw one rectangle around
   * repo + branch + remove).
   */
  flat?: boolean;
  /** Optional cmdk scorer override. Branch pickers pass `scoreBranch`. */
  filter?: (value: string, search: string, keywords?: string[]) => number;
  /** Optional hover tooltip for truncated labels or extra context. */
  tooltip?: string;
  /**
   * Optional muted prefix shown before the value in the trigger button.
   * Used by the branch chip to distinguish "current: <branch>" (no-op),
   * "will switch to: <branch>" (destructive) and "from: <branch>" (worktree
   * base) without depending on the user reading a tooltip.
   */
  prefix?: string;
};

/** Returns the active-state hover classes for the pill trigger button. */
function pillActiveClass(flat: boolean): string {
  if (flat) return "hover:bg-muted/60 cursor-pointer";
  return "hover:bg-muted hover:border-border cursor-pointer";
}

function PillCommandList({
  options,
  onSelect,
  setOpen,
  emptyMessage,
}: {
  options: PillOption[];
  onSelect: (value: string) => void;
  setOpen: (open: boolean) => void;
  emptyMessage: string;
}) {
  return (
    <CommandList>
      <CommandEmpty>{emptyMessage}</CommandEmpty>
      <CommandGroup>
        {options.map((option) => (
          <CommandItem
            key={option.value}
            value={option.value}
            keywords={[option.label, ...(option.keywords ?? [])]}
            onSelect={() => {
              onSelect(option.value);
              setOpen(false);
            }}
          >
            {option.renderLabel ? option.renderLabel() : option.label}
          </CommandItem>
        ))}
      </CommandGroup>
    </CommandList>
  );
}

/**
 * Builds the className for the pill trigger button. Extracted so the inline
 * trigger JSX stays compact (the Pill function is right at the complexity cap).
 */
function pillTriggerClass(disabled: boolean, flat: boolean, hasValue: boolean): string {
  return cn(
    "h-7 inline-flex items-center gap-1.5 rounded-md px-2.5 text-xs",
    flat ? "bg-transparent" : "border border-border/60 bg-muted/30",
    disabled ? "opacity-50 cursor-not-allowed" : pillActiveClass(flat),
    !hasValue && "text-muted-foreground",
  );
}

/**
 * Compact pill trigger that opens a popover with a search list. Auto-widths
 * to its content (no `w-full`, no chevron) so multiple pills can sit on one
 * line without overlapping or stretching to fill the row.
 */
export function Pill({
  icon,
  value,
  placeholder,
  options,
  onSelect,
  disabled = false,
  disabledReason,
  searchPlaceholder,
  emptyMessage,
  testId,
  onRefresh,
  refreshing,
  flat = false,
  filter,
  tooltip,
  prefix,
}: PillProps) {
  const [open, setOpenState] = useState(false);
  // Tracks the brief window after the popover closes during which the cursor
  // is still hovering the trigger. Without this, Radix's Tooltip becomes
  // uncontrolled the moment `open` flips to false and pops back open from the
  // lingering hover. Holding `open={false}` for ~200ms lets the user move
  // away naturally and prevents the tooltip from racing back in.
  const [suppressTooltip, setSuppressTooltip] = useState(false);
  const suppressTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const setOpen = useCallback((next: boolean) => {
    setOpenState((prev) => {
      if (prev && !next) {
        if (suppressTimerRef.current) clearTimeout(suppressTimerRef.current);
        setSuppressTooltip(true);
        suppressTimerRef.current = setTimeout(() => {
          setSuppressTooltip(false);
          suppressTimerRef.current = null;
        }, 200);
      }
      return next;
    });
  }, []);
  // Clear the suppression timer if the component unmounts while it's still
  // ticking, so the eventual `setSuppressTooltip(false)` doesn't fire on a
  // dead component.
  useEffect(
    () => () => {
      if (suppressTimerRef.current) {
        clearTimeout(suppressTimerRef.current);
        suppressTimerRef.current = null;
      }
    },
    [],
  );
  const hasValue = !!value;
  // Prefix only renders alongside a real value; an empty chip with placeholder
  // ("branch") doesn't need "current: " in front of it.
  const showPrefix = !!prefix && hasValue;
  const triggerButton = (
    <button
      type="button"
      disabled={disabled}
      data-testid={testId}
      className={pillTriggerClass(disabled, flat, hasValue)}
    >
      {icon}
      <span className="truncate max-w-[240px]">
        {showPrefix && <span className="text-muted-foreground">{prefix}</span>}
        {value || placeholder}
      </span>
    </button>
  );

  // A disabled <button> swallows pointer events, so wrap it in a span that
  // forwards hover to the tooltip trigger. Keep the Popover render path while
  // the popover is open even if `disabled` flips true mid-interaction (e.g.
  // refresh sets branchesLoading=true) — otherwise the popover unmounts and
  // the user loses their place.
  if (disabled && disabledReason && !open) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="inline-flex">{triggerButton}</span>
        </TooltipTrigger>
        <TooltipContent>{disabledReason}</TooltipContent>
      </Tooltip>
    );
  }

  const popover = (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        {tooltip ? <TooltipTrigger asChild>{triggerButton}</TooltipTrigger> : triggerButton}
      </PopoverTrigger>
      <PopoverContent className="w-[360px] p-0" align="start" portal={false}>
        <Command filter={filter}>
          <div className="flex items-center gap-1 px-2 pt-1">
            <CommandInput placeholder={searchPlaceholder} className="h-9 flex-1" />
            {onRefresh && <BranchRefreshButton onRefresh={onRefresh} refreshing={refreshing} />}
          </div>
          <PillCommandList
            options={options}
            onSelect={onSelect}
            setOpen={setOpen}
            emptyMessage={emptyMessage}
          />
        </Command>
      </PopoverContent>
    </Popover>
  );

  if (!tooltip) return popover;

  // Suppress the hover tooltip while the popover is open so they don't stack,
  // and for a brief window after it closes so a lingering hover doesn't pop
  // the tooltip back in (which Radix would otherwise do the moment we flip
  // from controlled-closed back to uncontrolled).
  const tooltipOpen = open || suppressTooltip ? false : undefined;
  return (
    <Tooltip open={tooltipOpen}>
      {popover}
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}

// --- Branch utilities ---

// Conventional default branches surfaced at the top of the dropdown when no
// search term is active. cmdk preserves option order on empty queries, so a
// stable sort here lifts main/master/develop above feature branches.
const PREFERRED_BRANCH_NAMES = ["main", "master", "develop"];

function branchPriority(b: Branch): number {
  const idx = PREFERRED_BRANCH_NAMES.indexOf(b.name);
  if (idx === -1) return PREFERRED_BRANCH_NAMES.length;
  return idx;
}

export function sortBranches(branches: Branch[]): Branch[] {
  return [...branches].sort((a, b) => {
    const pa = branchPriority(a);
    const pb = branchPriority(b);
    if (pa !== pb) return pa - pb;
    // Within the same priority bucket, locals before remotes — matches the
    // auto-select preference (`main` over `origin/main`).
    if (a.type !== b.type) return a.type === "local" ? -1 : 1;
    return 0;
  });
}

const BRANCH_SEGMENT_RE = /[/_.\-\s]+/;

export function buildBranchKeywords(name: string, remote?: string): string[] {
  const out = new Set<string>();
  out.add(name);
  const leafIdx = name.lastIndexOf("/");
  if (leafIdx >= 0) out.add(name.slice(leafIdx + 1));
  for (const seg of name.split(BRANCH_SEGMENT_RE)) {
    if (seg) out.add(seg);
  }
  if (remote) out.add(remote);
  return Array.from(out);
}

export function branchToOption(b: Branch): PillOption {
  // Remote branches keep their "origin/" prefix so they're distinguishable
  // from local branches with the same short name (e.g. "main" vs "origin/main").
  // Without the prefix, the dropdown shows two indistinguishable rows.
  const display = b.type === "remote" && b.remote ? `${b.remote}/${b.name}` : b.name;
  // `||` (not `??`) so an empty-string `remote` falls back too. Provider-backed
  // workspace repos (URL-added) list branches without a tracking remote, so the
  // backend sends `remote: ""`; `??` would render an invisible empty badge.
  const badge = b.type === "local" ? "local" : b.remote || "remote";
  return {
    value: display,
    label: display,
    keywords: buildBranchKeywords(b.name, b.remote),
    renderLabel: () => (
      <span className="flex min-w-0 flex-1 items-center justify-between gap-2">
        <span className="truncate" title={display}>
          {display}
        </span>
        <Badge variant="outline" className="text-xs shrink-0">
          {badge}
        </Badge>
      </span>
    ),
  };
}

export function computeBranchPlaceholder(
  hasRepo: boolean,
  loading: boolean,
  optionCount: number,
): string {
  if (!hasRepo) return "branch";
  if (loading) return "loading…";
  if (optionCount === 0) return "no branches";
  return "branch";
}
