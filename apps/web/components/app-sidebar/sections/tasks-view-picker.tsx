"use client";

import { useMemo, useState } from "react";
import { IconChevronDown, IconAdjustments, IconCheck } from "@tabler/icons-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { useAppStore } from "@/components/state-provider";
import { SidebarFilterPopover } from "@/components/task/sidebar-filter/sidebar-filter-popover";
import { cn } from "@/lib/utils";

const TRIGGER_BUTTON_CLASS = cn(
  "flex h-5 items-center justify-center rounded-sm px-1.5 cursor-pointer",
  "text-[11px] font-medium text-muted-foreground hover:text-foreground hover:bg-muted/60 transition-colors",
);

export function TasksViewPicker() {
  const views = useAppStore((s) => s.sidebarViews.views);
  const activeViewId = useAppStore((s) => s.sidebarViews.activeViewId);
  const draft = useAppStore((s) => s.sidebarViews.draft);
  const setActiveView = useAppStore((s) => s.setSidebarActiveView);
  const [filterOpen, setFilterOpen] = useState(false);

  const activeView = useMemo(
    () => views.find((v) => v.id === activeViewId) ?? views[0],
    [views, activeViewId],
  );
  const hasDraft = !!draft && draft.baseViewId === activeViewId;
  const activeLabel = activeView?.name ?? "All";

  return (
    <div className="flex items-center gap-0.5">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            type="button"
            data-testid="tasks-view-picker"
            className={cn(TRIGGER_BUTTON_CLASS, "max-w-[120px] gap-0.5")}
            aria-label={`View: ${activeLabel}`}
          >
            <span className="truncate">{activeLabel}</span>
            <IconChevronDown className="h-3 w-3 shrink-0 opacity-70" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-44">
          {views.map((view) => {
            const isActive = view.id === activeViewId;
            return (
              <DropdownMenuItem
                key={view.id}
                onSelect={() => setActiveView(view.id)}
                data-testid="sidebar-view-chip"
                data-view-id={view.id}
                data-active={isActive}
                aria-pressed={isActive}
                className="cursor-pointer gap-2 text-xs"
              >
                <IconCheck className={cn("h-3.5 w-3.5", isActive ? "opacity-100" : "opacity-0")} />
                <span className="truncate">{view.name}</span>
              </DropdownMenuItem>
            );
          })}
        </DropdownMenuContent>
      </DropdownMenu>
      <SidebarFilterPopover
        open={filterOpen}
        onOpenChange={setFilterOpen}
        trigger={
          <button
            type="button"
            data-testid="sidebar-filter-gear"
            aria-label="Filters and sort"
            className={cn(TRIGGER_BUTTON_CLASS, "relative h-5 w-5 px-0")}
          >
            <IconAdjustments className="h-3.5 w-3.5" />
            {hasDraft && (
              <span
                data-testid="sidebar-filter-gear-indicator"
                aria-label="Unsaved filter changes"
                className="absolute right-0.5 top-0.5 h-1 w-1 rounded-full bg-amber-500"
              />
            )}
          </button>
        }
      />
    </div>
  );
}
