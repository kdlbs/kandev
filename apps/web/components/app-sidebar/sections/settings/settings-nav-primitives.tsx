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
  label: string;
  icon?: TablerIcon;
  /** When the group itself has a destination, the label area is also a link. */
  href?: string;
  isActive?: boolean;
  defaultExpanded?: boolean;
  depth?: number;
  children: ReactNode;
};

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
  const toggle = () => setExpanded((v) => !v);

  const labelInner = (
    <>
      {Icon && <Icon className="h-3.5 w-3.5 shrink-0" />}
      <span className="flex-1 truncate">{label}</span>
    </>
  );

  return (
    <div>
      <div
        className={cn(
          "flex items-center gap-1 rounded-md",
          isActive ? ACTIVE_CLASS : INACTIVE_CLASS,
          paddingClass,
        )}
      >
        {href ? (
          <Link
            href={href}
            className="flex flex-1 min-w-0 items-center gap-2 py-1.5 text-[13px] font-medium cursor-pointer"
          >
            {labelInner}
          </Link>
        ) : (
          <button
            type="button"
            onClick={toggle}
            className="flex flex-1 min-w-0 items-center gap-2 py-1.5 text-[13px] font-medium cursor-pointer text-left"
          >
            {labelInner}
          </button>
        )}
        <button
          type="button"
          onClick={toggle}
          aria-label={expanded ? `Collapse ${label}` : `Expand ${label}`}
          aria-expanded={expanded}
          className="shrink-0 flex h-5 w-5 items-center justify-center text-muted-foreground/60 hover:text-foreground/80 cursor-pointer transition-colors"
        >
          <IconChevronRight
            className={cn("h-3 w-3 transition-transform", expanded && "rotate-90")}
          />
        </button>
      </div>
      {expanded && <div className="flex flex-col gap-0.5">{children}</div>}
    </div>
  );
}
