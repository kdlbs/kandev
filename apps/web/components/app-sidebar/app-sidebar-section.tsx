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
  icon: TablerIcon;
  children: React.ReactNode;
  /** Optional control rendered between the label and the collapse chevron. */
  headerAction?: React.ReactNode;
  /** Fills remaining sidebar height when expanded. Parent must be a flex column. */
  grow?: boolean;
};

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
  const handleToggle = () => toggleSection(id);

  return (
    <div className={cn(growExpanded && "flex-1 min-h-0 flex flex-col")}>
      <div className="group/section flex items-center px-2 h-7 shrink-0">
        <button
          type="button"
          onClick={handleToggle}
          className="flex min-w-0 flex-1 items-center text-left cursor-pointer text-foreground/70 hover:text-foreground transition-colors"
          aria-expanded={expanded}
        >
          <span className="text-[11px] font-semibold uppercase tracking-wider truncate">
            {label}
          </span>
        </button>
        {expanded && headerAction && (
          <div className="shrink-0 mr-1 flex items-center">{headerAction}</div>
        )}
        <button
          type="button"
          onClick={handleToggle}
          tabIndex={-1}
          aria-hidden="true"
          className="shrink-0 flex h-5 w-5 items-center justify-center text-muted-foreground/60 hover:text-foreground/70 cursor-pointer transition-colors"
        >
          <IconChevronRight
            className={cn("h-3.5 w-3.5 transition-transform", expanded && "rotate-90")}
          />
        </button>
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
