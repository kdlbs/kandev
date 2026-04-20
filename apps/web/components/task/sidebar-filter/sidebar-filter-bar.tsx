"use client";

import { useMemo, useState } from "react";
import { IconAdjustments } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { useRegisterCommands } from "@/hooks/use-register-commands";
import type { CommandItem } from "@/lib/commands/types";
import { SidebarViewChips } from "./sidebar-view-chips";
import { SidebarFilterPopover } from "./sidebar-filter-popover";

export function SidebarFilterBar() {
  const [open, setOpen] = useState(false);
  const draft = useAppStore((s) => s.sidebarViews.draft);
  const activeViewId = useAppStore((s) => s.sidebarViews.activeViewId);
  const views = useAppStore((s) => s.sidebarViews.views);
  const setActiveView = useAppStore((s) => s.setSidebarActiveView);
  const hasDraft = !!draft && draft.baseViewId === activeViewId;

  const commands = useMemo<CommandItem[]>(() => {
    const list: CommandItem[] = [
      {
        id: "sidebar-open-filter",
        label: "Open sidebar filters",
        group: "Sidebar",
        keywords: ["filter", "sort", "group", "view", "sidebar"],
        action: () => setOpen(true),
      },
    ];
    for (const view of views) {
      list.push({
        id: `sidebar-switch-view-${view.id}`,
        label: `Switch sidebar view: ${view.name}`,
        group: "Sidebar",
        keywords: ["view", "switch", "sidebar", view.name.toLowerCase()],
        action: () => setActiveView(view.id),
      });
    }
    return list;
  }, [views, setActiveView]);
  useRegisterCommands(commands);

  return (
    <div data-testid="sidebar-filter-bar" className="flex items-center gap-1 border-b px-2 py-1">
      <SidebarViewChips />
      <SidebarFilterPopover
        open={open}
        onOpenChange={setOpen}
        trigger={
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="h-6 w-6 shrink-0 cursor-pointer"
            data-testid="sidebar-filter-gear"
            aria-label="Sidebar filters"
          >
            <IconAdjustments className="h-4 w-4" />
            {hasDraft && (
              <span
                className="absolute right-1 top-1 h-1.5 w-1.5 rounded-full bg-amber-500"
                data-testid="sidebar-filter-gear-indicator"
              />
            )}
          </Button>
        }
      />
    </div>
  );
}
