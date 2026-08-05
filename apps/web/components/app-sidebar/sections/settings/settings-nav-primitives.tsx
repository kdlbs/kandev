"use client";

import Link from "@/components/routing/app-link";
import type { ComponentType, ReactNode } from "react";
import { cn } from "@/lib/utils";
import { SIDEBAR_ITEM_ACTIVE, SIDEBAR_ITEM_INACTIVE } from "../../app-sidebar-constants";

// Same active treatment as the task list rows (task-item.tsx): a primary-tinted
// fill, layered on the sidebar's shared left accent bar.
const ACTIVE_CLASS = cn(SIDEBAR_ITEM_ACTIVE, "bg-primary/10 hover:bg-primary/15");
const INACTIVE_CLASS = SIDEBAR_ITEM_INACTIVE;

type SettingsLeafProps = {
  href: string;
  label: string;
  icon?: ComponentType<{ className?: string }>;
  /** Pre-rendered leading visual (e.g. AgentLogo). Takes precedence over `icon`. */
  leadingIcon?: ReactNode;
  labelSuffix?: ReactNode;
  isActive: boolean;
  /** Nesting level — used to add left padding. */
  depth?: number;
};

const LEAF_DEPTH_PADDING = ["px-2.5", "pl-7 pr-2.5", "pl-10 pr-2.5", "pl-[52px] pr-2.5"] as const;
const NAV_FOCUS_CLASS =
  "outline-none focus-visible:ring-1 focus-visible:ring-primary/50 focus-visible:ring-offset-1";

function clampDepth(depth: number, max: number): number {
  if (depth < 0) return 0;
  if (depth > max) return max;
  return depth;
}

export function SettingsLeaf({
  href,
  label,
  icon: Icon,
  leadingIcon,
  labelSuffix,
  isActive,
  depth = 0,
}: SettingsLeafProps) {
  return (
    <Link
      href={href}
      // Exposed so a test can count the active rows: exactly one row may claim
      // the current location, which is what group headers linking to their first
      // leaf used to break.
      data-active={isActive ? "true" : undefined}
      className={cn(
        "flex items-center gap-2 py-1.5 text-[13px] font-medium rounded-md cursor-pointer",
        NAV_FOCUS_CLASS,
        LEAF_DEPTH_PADDING[clampDepth(depth, LEAF_DEPTH_PADDING.length - 1)],
        isActive ? ACTIVE_CLASS : INACTIVE_CLASS,
      )}
    >
      {leadingIcon ?? (Icon && <Icon className="h-3.5 w-3.5 shrink-0" />)}
      <span className="flex-1 truncate">{label}</span>
      {labelSuffix}
    </Link>
  );
}

/**
 * A static group label. Not a link, not a button: the menu is exactly two
 * levels (header → page) and headers have no page or expand state of their own.
 */
export function SettingsSectionHeader({ label }: { label: string }) {
  return (
    <div className="px-2.5 pb-1 pt-3 text-[10px] font-semibold uppercase tracking-[0.08em] text-muted-foreground/80 first:pt-1">
      {label}
    </div>
  );
}
