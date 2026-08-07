"use client";

import Link from "@/components/routing/app-link";
import { IconChevronRight } from "@tabler/icons-react";
import type { ComponentType, ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Collapsible, CollapsibleContent } from "@kandev/ui/collapsible";
import { RecordDot } from "@/components/settings/record-dot";
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
  /** Row built from the user's own data — rendered with a lighter label. */
  isRecord?: boolean;
};

const LEAF_DEPTH_PADDING = ["px-2.5", "pl-7 pr-2.5", "pl-10 pr-2.5", "pl-[52px] pr-2.5"] as const;
const BRANCH_DEPTH_PADDING = ["pl-2.5 pr-1", "pl-7 pr-1", "pl-10 pr-1", "pl-[52px] pr-1"] as const;
const NAV_FOCUS_CLASS =
  "outline-none focus-visible:ring-1 focus-visible:ring-primary/50 focus-visible:ring-offset-1";

function clampDepth(depth: number, max: number): number {
  if (depth < 0) return 0;
  if (depth > max) return max;
  return depth;
}

const GLYPH_CLASS = "h-3.5 w-3.5 shrink-0";

/**
 * The mark on a row built from the user's own data.
 *
 * The dot is the shared one the Agents page puts in front of a profile, so a
 * profile is marked the same way in the menu as on its own page — including
 * when it is disabled. It is centred in a full glyph box rather than rendered
 * loose: at 6px it would otherwise sit narrower than every tabler glyph around
 * it and pull its label out of the column.
 */
export function RecordGlyph({ enabled }: { enabled?: boolean }) {
  return (
    <span className={cn(GLYPH_CLASS, "flex items-center justify-center")} aria-hidden="true">
      <RecordDot enabled={enabled} />
    </span>
  );
}

/**
 * A row's icon, title and trailing detail.
 *
 * The count badge sits immediately after the title rather than flush against
 * the row's right edge: right-aligned, it read as a column of unrelated numbers
 * running down the menu, and on a branch row it collided with the chevron. The
 * `flex-1` lives on this wrapper, not the title, so the title still truncates
 * and a branch's chevron still gets pushed to the edge.
 *
 * Every row opens a glyph box, and a row with nothing to put in it keeps the box
 * empty. That is a fallback rather than a layout the menu relies on — records
 * carry `RecordDot` — but it costs one element and keeps a future glyphless row
 * from pulling its label 22px (glyph plus gap) left of the column its
 * neighbours sit in.
 */
function NavRowLabel({
  label,
  labelSuffix,
  icon: Icon,
  leadingIcon,
}: {
  label: string;
  labelSuffix?: ReactNode;
  icon?: ComponentType<{ className?: string }>;
  leadingIcon?: ReactNode;
}) {
  const visual = leadingIcon ?? (Icon && <Icon className={GLYPH_CLASS} />);
  return (
    <>
      {visual ?? <span className={GLYPH_CLASS} aria-hidden="true" />}
      <span className="flex min-w-0 flex-1 items-center gap-1.5">
        <span className="truncate">{label}</span>
        {labelSuffix}
      </span>
    </>
  );
}

export function SettingsLeaf({
  href,
  label,
  icon: Icon,
  leadingIcon,
  labelSuffix,
  isActive,
  depth = 0,
  isRecord = false,
}: SettingsLeafProps) {
  return (
    <Link
      href={href}
      // Exposed so a test can count the active rows: exactly one row may claim
      // the current location, which is what group headers linking to their first
      // leaf used to break.
      data-active={isActive ? "true" : undefined}
      data-record={isRecord ? "true" : undefined}
      className={cn(
        "flex items-center gap-2 py-1.5 text-[13px] rounded-md cursor-pointer",
        isRecord ? "font-normal" : "font-medium",
        NAV_FOCUS_CLASS,
        LEAF_DEPTH_PADDING[clampDepth(depth, LEAF_DEPTH_PADDING.length - 1)],
        isActive ? ACTIVE_CLASS : INACTIVE_CLASS,
      )}
    >
      <NavRowLabel
        label={label}
        labelSuffix={labelSuffix}
        icon={Icon}
        leadingIcon={leadingIcon}
      />
    </Link>
  );
}

type SettingsBranchProps = {
  label: string;
  icon?: ComponentType<{ className?: string }>;
  /** Pre-rendered leading visual (e.g. AgentLogo). Takes precedence over `icon`. */
  leadingIcon?: ReactNode;
  labelSuffix?: ReactNode;
  /**
   * The branch's own page. Omitted for a node that only discloses — an agent,
   * whose `/settings/agents/<name>` route redirects back to the index.
   */
  href?: string;
  /** True only when this row *is* the current page, never when it contains it. */
  isActive?: boolean;
  expanded: boolean;
  onToggle: () => void;
  depth?: number;
  /** Row built from the user's own data — rendered with a lighter label. */
  isRecord?: boolean;
  children: ReactNode;
};

/**
 * A settings row that also holds sub-rows, used only by the tree modes.
 *
 * Expansion is always controlled: which branches are open is a property of the
 * chosen mode (one path for accordion, a persisted set for persistent), so it
 * belongs to `useSettingsMenuExpansion` rather than to each row. A branch with
 * an `href` splits its header — the label navigates, the chevron expands — so
 * reaching a workspace's own page never costs you the branch you just opened.
 */
export function SettingsBranch({
  label,
  icon: Icon,
  leadingIcon,
  labelSuffix,
  href,
  isActive = false,
  expanded,
  onToggle,
  depth = 0,
  isRecord = false,
  children,
}: SettingsBranchProps) {
  const { t } = useTranslation();
  const labelInner = (
    <NavRowLabel
      label={label}
      labelSuffix={labelSuffix}
      icon={Icon}
      leadingIcon={leadingIcon}
    />
  );
  const labelClass = cn(
    "flex flex-1 min-w-0 items-center gap-2 py-1.5 text-[13px] cursor-pointer text-left",
    isRecord ? "font-normal" : "font-medium",
    NAV_FOCUS_CLASS,
  );
  // On the row element rather than the label, so an href-less branch (an agent,
  // whose label is a button) carries it too.
  const recordAttribute = isRecord ? "true" : undefined;

  return (
    <Collapsible open={expanded}>
      <div
        data-record={recordAttribute}
        className={cn(
          "flex items-center gap-1 rounded-md",
          isActive ? ACTIVE_CLASS : INACTIVE_CLASS,
          BRANCH_DEPTH_PADDING[clampDepth(depth, BRANCH_DEPTH_PADDING.length - 1)],
        )}
      >
        {href ? (
          <Link href={href} data-active={isActive ? "true" : undefined} className={labelClass}>
            {labelInner}
          </Link>
        ) : (
          <button type="button" onClick={onToggle} className={labelClass}>
            {labelInner}
          </button>
        )}
        <button
          type="button"
          onClick={onToggle}
          aria-label={
            expanded ? t("sidebar:collapseGroup", { label }) : t("sidebar:expandGroup", { label })
          }
          aria-expanded={expanded}
          className={cn(
            "shrink-0 flex h-5 w-5 items-center justify-center text-muted-foreground/60 hover:text-foreground/80 cursor-pointer transition-colors",
            NAV_FOCUS_CLASS,
          )}
        >
          <IconChevronRight
            className={cn("h-3 w-3 transition-transform", expanded && "rotate-90")}
          />
        </button>
      </div>
      <CollapsibleContent className="sidebar-section-content">
        <div className="flex flex-col gap-0.5">{children}</div>
      </CollapsibleContent>
    </Collapsible>
  );
}

/**
 * A static group label. Not a link, not a button: the section headers are the
 * menu's fixed spine in every mode — the tree modes grow branches under the
 * rows, never under a header, which has no page or expand state of its own.
 */
export function SettingsSectionHeader({ label }: { label: string }) {
  return (
    <div className="px-2.5 pb-1 pt-3 text-[10px] font-semibold uppercase tracking-[0.08em] text-muted-foreground/80 first:pt-1">
      {label}
    </div>
  );
}
