"use client";

import Link from "next/link";
import { IconChevronRight } from "@tabler/icons-react";
import type { Icon as TablerIcon } from "@tabler/icons-react";
import { useState, type ReactNode } from "react";
import { cn } from "@/lib/utils";

const ACTIVE_CLASS = "bg-accent text-foreground";
const INACTIVE_CLASS = "text-foreground/80 hover:bg-muted/60";

type SettingsLeafProps = {
  href: string;
  label: string;
  icon?: TablerIcon;
  isActive: boolean;
  /** Nesting level — used to add left padding. */
  depth?: number;
};

const LEAF_DEPTH_PADDING = ["px-2.5", "pl-7 pr-2.5", "pl-10 pr-2.5"] as const;
const GROUP_DEPTH_PADDING = ["pl-2.5 pr-1", "pl-7 pr-1", "pl-10 pr-1"] as const;

function clampDepth(depth: number, max: number): number {
  if (depth < 0) return 0;
  if (depth > max) return max;
  return depth;
}

/** A clickable link row inside the Settings tree. */
export function SettingsLeaf({ href, label, icon: Icon, isActive, depth = 0 }: SettingsLeafProps) {
  return (
    <Link
      href={href}
      className={cn(
        "flex items-center gap-2 py-1.5 text-[13px] font-medium rounded-md cursor-pointer",
        LEAF_DEPTH_PADDING[clampDepth(depth, LEAF_DEPTH_PADDING.length - 1)],
        isActive ? ACTIVE_CLASS : INACTIVE_CLASS,
      )}
    >
      {Icon && <Icon className="h-3.5 w-3.5 shrink-0" />}
      <span className="flex-1 truncate">{label}</span>
    </Link>
  );
}

type SettingsGroupProps = {
  /** Group toggle label. */
  label: string;
  /** Optional leading icon. */
  icon?: TablerIcon;
  /** When the group itself has a destination, the toggle is also a link. */
  href?: string;
  isActive?: boolean;
  defaultExpanded?: boolean;
  depth?: number;
  children: ReactNode;
};

/**
 * Collapsible nested group. Toggling the chevron expands/collapses children;
 * if `href` is given, clicking the label area navigates to that route.
 */
export function SettingsGroup({
  label,
  icon: Icon,
  href,
  isActive,
  defaultExpanded = false,
  depth = 0,
  children,
}: SettingsGroupProps) {
  const [expanded, setExpanded] = useState(defaultExpanded);

  const paddingClass = GROUP_DEPTH_PADDING[clampDepth(depth, GROUP_DEPTH_PADDING.length - 1)];

  return (
    <div>
      <div
        className={cn(
          "flex items-center gap-1 rounded-md",
          isActive ? ACTIVE_CLASS : INACTIVE_CLASS,
          paddingClass,
        )}
      >
        <button
          type="button"
          onClick={() => setExpanded((v) => !v)}
          className="cursor-pointer p-0.5 -ml-0.5"
          aria-label={expanded ? `Collapse ${label}` : `Expand ${label}`}
        >
          <IconChevronRight
            className={cn("h-3 w-3 transition-transform", expanded && "rotate-90")}
          />
        </button>
        {href ? (
          <Link
            href={href}
            className="flex flex-1 items-center gap-2 py-1.5 text-[13px] font-medium cursor-pointer"
          >
            {Icon && <Icon className="h-3.5 w-3.5 shrink-0" />}
            <span className="flex-1 truncate">{label}</span>
          </Link>
        ) : (
          <button
            type="button"
            onClick={() => setExpanded((v) => !v)}
            className="flex flex-1 items-center gap-2 py-1.5 text-[13px] font-medium cursor-pointer text-left"
          >
            {Icon && <Icon className="h-3.5 w-3.5 shrink-0" />}
            <span className="flex-1 truncate">{label}</span>
          </button>
        )}
      </div>
      {expanded && <div className="flex flex-col gap-0.5">{children}</div>}
    </div>
  );
}
