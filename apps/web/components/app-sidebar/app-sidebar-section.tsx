"use client";

import { IconChevronRight } from "@tabler/icons-react";
import type { Icon as TablerIcon } from "@tabler/icons-react";
import { Collapsible, CollapsibleContent } from "@kandev/ui/collapsible";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { cn } from "@/lib/utils";

type AppSidebarSectionProps = {
  id: string;
  label: string;
  collapsed: boolean;
  icon: TablerIcon;
  children: React.ReactNode;
  /** Optional control rendered between the label and the collapse chevron. */
  headerAction?: React.ReactNode;
  /** By default header actions render only while the section accordion is open.
   *  "always" keeps them visible while the accordion is closed, but has no
   *  effect when the sidebar itself is in collapsed/rail mode. */
  headerActionVisibility?: "expanded" | "always";
  /** Fills remaining sidebar height when expanded. Parent must be a flex column. */
  grow?: boolean;
  /** Initial expansion when the persisted section map does not yet contain this id. */
  defaultExpanded?: boolean;
};

type SectionHeaderProps = {
  label: string;
  expanded: boolean;
  headerAction?: React.ReactNode;
  headerActionVisibility: "expanded" | "always";
  onToggle: () => void;
};

function SectionHeader({
  label,
  expanded,
  headerAction,
  headerActionVisibility,
  onToggle,
}: SectionHeaderProps) {
  const showHeaderAction = !!headerAction && (expanded || headerActionVisibility === "always");

  return (
    <div className="group/section flex items-center px-2 h-7 shrink-0">
      <button
        type="button"
        onClick={onToggle}
        className="flex min-w-0 flex-1 items-center text-left cursor-pointer text-foreground/70 hover:text-foreground transition-colors"
        aria-expanded={expanded}
      >
        <span className="text-[11px] font-semibold uppercase tracking-wider truncate">{label}</span>
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

export function AppSidebarSection({
  id,
  label,
  collapsed,
  icon: Icon,
  children,
  headerAction,
  headerActionVisibility = "expanded",
  grow,
  defaultExpanded = false,
}: AppSidebarSectionProps) {
  const expanded = useAppStore((s) => s.appSidebar.sectionExpanded[id] ?? defaultExpanded);
  const toggleSection = useAppStore((s) => s.toggleAppSidebarSection);
  const setCollapsed = useAppStore((s) => s.setAppSidebarCollapsed);

  const railButton = (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          className="flex h-9 w-9 mx-auto items-center justify-center rounded-md text-foreground/70 hover:bg-muted/60 cursor-pointer"
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
      <div className={cn(!collapsed && expanded && "flex-1 min-h-0 flex flex-col")}>
        {collapsed ? (
          railButton
        ) : (
          <SectionHeader
            label={label}
            expanded={expanded}
            headerAction={headerAction}
            headerActionVisibility={headerActionVisibility}
            onToggle={handleToggle}
          />
        )}
        {expanded && (
          <div
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
      <SectionHeader
        label={label}
        expanded={expanded}
        headerAction={headerAction}
        headerActionVisibility={headerActionVisibility}
        onToggle={handleToggle}
      />
      <CollapsibleContent className="sidebar-section-content">
        <div className="flex flex-col gap-0.5">{children}</div>
      </CollapsibleContent>
    </Collapsible>
  );
}
