"use client";

import { IconChevronRight } from "@tabler/icons-react";
import type { Icon as TablerIcon } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { cn } from "@/lib/utils";

type AppSidebarSectionProps = {
  id: string;
  label: string;
  collapsed: boolean;
  /** Icon used as the collapsed-mode label. */
  icon: TablerIcon;
  children: React.ReactNode;
  /** Optional right-aligned action shown in the header when expanded. */
  headerAction?: React.ReactNode;
  /**
   * When true and expanded, the section wrapper + body get `flex-1 min-h-0` so
   * it fills remaining sidebar height. Parent must be a flex column.
   */
  grow?: boolean;
};

/**
 * Reusable collapsible section primitive for the AppSidebar.
 *
 * Reads/writes per-section expanded state via the store. When the sidebar is
 * fully collapsed (icon-rail mode) we render the icon as a tooltip target and
 * clicking it expands the sidebar AND the section.
 */
export function AppSidebarSection({
  id,
  label,
  collapsed,
  icon: Icon,
  children,
  headerAction,
  grow,
}: AppSidebarSectionProps) {
  const expanded = useAppStore((s) => s.appSidebar.sectionExpanded[id] ?? false);
  const toggleSection = useAppStore((s) => s.toggleAppSidebarSection);
  const setCollapsed = useAppStore((s) => s.setAppSidebarCollapsed);

  if (collapsed) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            className="flex h-9 w-9 mx-auto items-center justify-center rounded-md text-foreground/70 hover:bg-muted/60 cursor-pointer"
            onClick={() => {
              setCollapsed(false);
              if (!expanded) toggleSection(id);
            }}
            aria-label={label}
          >
            <Icon className="h-4 w-4" />
          </button>
        </TooltipTrigger>
        <TooltipContent side="right">{label}</TooltipContent>
      </Tooltip>
    );
  }

  const growExpanded = grow && expanded;

  return (
    <div className={cn(growExpanded && "flex-1 min-h-0 flex flex-col")}>
      <div className="flex items-center justify-between px-2.5 py-1.5 shrink-0">
        <button
          type="button"
          onClick={() => toggleSection(id)}
          className="flex items-center gap-1 cursor-pointer"
        >
          <IconChevronRight
            className={cn(
              "h-3 w-3 text-muted-foreground/60 transition-transform",
              expanded && "rotate-90",
            )}
          />
          <span className="text-[10px] font-medium uppercase tracking-widest font-mono text-muted-foreground/60">
            {label}
          </span>
        </button>
        {headerAction}
      </div>
      {expanded && (
        <div
          className={cn(
            "flex flex-col gap-0.5 sidebar-fade-in-2",
            growExpanded && "flex-1 min-h-0",
          )}
        >
          {children}
        </div>
      )}
    </div>
  );
}
