"use client";

import { cn } from "@/lib/utils";
import { useAppStore } from "@/components/state-provider";

export function SidebarViewChips() {
  const views = useAppStore((s) => s.sidebarViews.views);
  const activeViewId = useAppStore((s) => s.sidebarViews.activeViewId);
  const setActive = useAppStore((s) => s.setSidebarActiveView);

  return (
    <div
      className="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto"
      data-testid="sidebar-view-chip-row"
    >
      {views.map((view) => {
        const active = view.id === activeViewId;
        return (
          <button
            type="button"
            key={view.id}
            onClick={() => setActive(view.id)}
            data-testid="sidebar-view-chip"
            data-view-id={view.id}
            data-active={active}
            data-builtin={view.isBuiltIn ? "true" : "false"}
            className={cn(
              "h-6 shrink-0 cursor-pointer rounded-md border px-2 text-[11px] transition-colors",
              active
                ? "border-primary/40 bg-primary/10 text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground",
            )}
            title={view.name}
          >
            <span className="truncate max-w-[120px]">{view.name}</span>
          </button>
        );
      })}
    </div>
  );
}
