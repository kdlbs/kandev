"use client";

import type { RefObject } from "react";
import { IconChevronRight } from "@tabler/icons-react";
import type { DestinationIcon } from "@/lib/navigation/types";
import { Collapsible, CollapsibleContent } from "@kandev/ui/collapsible";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { cn } from "@/lib/utils";

type AppSidebarSectionProps = {
  id: string;
  presentation?: "section" | "navigation";
  label: string;
  collapsed: boolean;
  icon: DestinationIcon;
  children: React.ReactNode;
  /** Optional control rendered between the label and the collapse chevron. */
  headerAction?: React.ReactNode;
  /** Rendered beside the label only while the accordion is shut. A section that
   *  starts folded still has to say how much it is hiding, or it reads as empty
   *  and nobody opens it. Muted by the header, so callers pass content, not
   *  colour. */
  collapsedSummary?: React.ReactNode;
  /** By default header actions render only while the section accordion is open.
   *  "always" keeps them visible while the accordion is closed, but has no
   *  effect when the sidebar itself is in collapsed/rail mode. */
  headerActionVisibility?: "expanded" | "always";
  /** Fills remaining sidebar height when expanded. Parent must be a flex column. */
  grow?: boolean;
  /** Initial expansion when the persisted section map does not yet contain this id. */
  defaultExpanded?: boolean;
  /** Stable focus target for dialogs opened from this section. */
  headerRef?: RefObject<HTMLButtonElement | null>;
};

type SectionHeaderProps = {
  id: string;
  label: string;
  expanded: boolean;
  headerAction?: React.ReactNode;
  headerActionVisibility: "expanded" | "always";
  collapsedSummary?: React.ReactNode;
  onToggle: () => void;
  headerRef?: RefObject<HTMLButtonElement | null>;
};

function SectionHeader({
  id,
  label,
  expanded,
  headerAction,
  headerActionVisibility,
  collapsedSummary,
  onToggle,
  headerRef,
}: SectionHeaderProps) {
  const showHeaderAction = !!headerAction && (expanded || headerActionVisibility === "always");

  return (
    <div className="group/section flex items-center px-2 min-h-9 shrink-0 [@media(pointer:coarse)]:min-h-11">
      <button
        ref={headerRef}
        type="button"
        onClick={onToggle}
        className="flex min-h-7 [@media(pointer:coarse)]:min-h-11 min-w-0 flex-1 items-center gap-1.5 text-left cursor-pointer text-foreground/70 hover:text-foreground transition-colors"
        aria-expanded={expanded}
        aria-controls={`sidebar-section-${id}`}
      >
        <span className="text-[11px] font-semibold uppercase tracking-wider truncate">{label}</span>
        {!expanded && collapsedSummary != null && (
          <span
            className="shrink-0 text-[11px] font-normal tabular-nums text-muted-foreground/70"
            data-testid="sidebar-section-collapsed-summary"
          >
            {collapsedSummary}
          </span>
        )}
      </button>
      {showHeaderAction && <div className="shrink-0 mr-1 flex items-center">{headerAction}</div>}
      <button
        type="button"
        onClick={onToggle}
        tabIndex={-1}
        aria-hidden="true"
        className="shrink-0 flex h-5 w-5 items-center justify-center text-muted-foreground/60 hover:text-foreground/70 cursor-pointer transition-colors"
      >
        <IconChevronRight
          className={cn("h-3.5 w-3.5 transition-transform", expanded && "rotate-90")}
        />
      </button>
    </div>
  );
}

function NavigationSectionHeader({
  label,
  expanded,
  onToggle,
  headerRef,
  headerAction,
  headerActionVisibility,
  collapsedSummary,
  icon: Icon,
  id,
}: SectionHeaderProps & { icon: DestinationIcon }) {
  return (
    <div className="flex min-w-0 items-center gap-1">
      <button
        ref={headerRef}
        type="button"
        onClick={onToggle}
        aria-expanded={expanded}
        aria-controls={`sidebar-section-${id}`}
        className="flex h-9 min-w-0 flex-1 cursor-pointer items-center gap-2.5 rounded-md px-2.5 text-left text-[13px] font-medium text-foreground/80 transition-colors hover:bg-muted/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring [@media(pointer:coarse)]:min-h-11"
      >
        <Icon className="size-4 shrink-0" aria-hidden="true" />
        <span className="flex-1 truncate">{label}</span>
        {!expanded && collapsedSummary != null && (
          <span
            data-testid="sidebar-section-collapsed-summary"
            className="text-[11px] tabular-nums text-muted-foreground"
          >
            {collapsedSummary}
          </span>
        )}
        <IconChevronRight
          className={cn(
            "size-3.5 shrink-0 text-muted-foreground transition-transform",
            expanded && "rotate-90",
          )}
          aria-hidden="true"
        />
      </button>
      {headerAction && (expanded || headerActionVisibility === "always") && headerAction}
    </div>
  );
}

export function AppSidebarSection({
  id,
  presentation = "section",
  label,
  collapsed,
  icon: Icon,
  children,
  headerAction,
  headerActionVisibility = "expanded",
  collapsedSummary,
  grow,
  defaultExpanded = false,
  headerRef,
}: AppSidebarSectionProps) {
  const expanded = useAppStore((s) => s.appSidebar.sectionExpanded[id] ?? defaultExpanded);
  const toggleSection = useAppStore((s) => s.toggleAppSidebarSection);
  const setCollapsed = useAppStore((s) => s.setAppSidebarCollapsed);

  const railButton = (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          ref={headerRef}
          type="button"
          className="flex h-9 w-9 [@media(pointer:coarse)]:size-11 mx-auto items-center justify-center rounded-md text-foreground/70 hover:bg-muted/60 cursor-pointer"
          onClick={() => {
            setCollapsed(false);
            if (!expanded) toggleSection(id, defaultExpanded);
          }}
          aria-label={label}
        >
          <Icon className="h-4 w-4" />
        </button>
      </TooltipTrigger>
      <TooltipContent side="right">{label}</TooltipContent>
    </Tooltip>
  );

  if (collapsed && !grow) return railButton;

  const handleToggle = () => toggleSection(id, defaultExpanded);

  const Header = presentation === "navigation" ? NavigationSectionHeader : SectionHeader;
  const header = (
    <Header
      id={id}
      icon={Icon}
      label={label}
      expanded={expanded}
      headerAction={headerAction}
      headerActionVisibility={headerActionVisibility}
      collapsedSummary={collapsedSummary}
      onToggle={handleToggle}
      headerRef={headerRef}
    />
  );

  // The grow section (Tasks) absorbs remaining vertical space and scrolls
  // internally, so it stays flex-driven rather than animating to content
  // height like the fixed-size sections below.
  //
  // In rail mode its children stay mounted but CSS-hidden: this section holds
  // the (potentially huge) task list, and unmounting it made collapsing the
  // sidebar jank — React tore down every task row in the same frame the close
  // animation started. Hiding is O(1) on close and makes reopening instant.
  // The children div keeps the same position in the tree across the toggle so
  // React preserves it instead of remounting.
  if (grow) {
    return (
      <div
        className={cn(
          !collapsed && "border-t border-border/60 pt-2",
          !collapsed && expanded && "flex-1 min-h-0 flex flex-col",
        )}
      >
        {collapsed ? railButton : header}
        {expanded && (
          <div
            id={`sidebar-section-${id}`}
            className={cn(
              "flex flex-col gap-0.5",
              collapsed ? "hidden" : "flex-1 min-h-0 sidebar-fade-in",
            )}
          >
            {children}
          </div>
        )}
      </div>
    );
  }

  return (
    <Collapsible open={expanded}>
      {header}
      <CollapsibleContent id={`sidebar-section-${id}`} className="sidebar-section-content">
        <div className={cn("flex flex-col gap-0.5", presentation === "navigation" && "ml-4 pl-2")}>
          {children}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}
