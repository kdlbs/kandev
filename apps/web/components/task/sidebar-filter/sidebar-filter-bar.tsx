"use client";

import { useMemo, useRef } from "react";
import { IconAdjustments, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { toast } from "sonner";
import { useAppStore } from "@/components/state-provider";
import { useRegisterCommands } from "@/hooks/use-register-commands";
import type { CommandItem } from "@/lib/commands/types";
import { cn } from "@/lib/utils";
import { SidebarViewChips } from "./sidebar-view-chips";
import { SidebarFilterPopover } from "./sidebar-filter-popover";
import { useSidebarViewPopover } from "./use-sidebar-view-popover";

export function SidebarFilterBar() {
  const filterTriggerRef = useRef<HTMLButtonElement>(null);
  const draft = useAppStore((s) => s.sidebarViews.draft);
  const activeViewId = useAppStore((s) => s.sidebarViews.activeViewId);
  const views = useAppStore((s) => s.sidebarViews.views);
  const setActiveView = useAppStore((s) => s.setSidebarActiveView);
  const hasDraft = !!draft && draft.baseViewId === activeViewId;
  const {
    open,
    onOpenChange,
    startNewView,
    renameRequestedViewId,
    consumeRenameRequest,
    newViewDisabledReason,
  } = useSidebarViewPopover();

  const commands = useMemo<CommandItem[]>(() => {
    const list: CommandItem[] = [
      {
        id: "sidebar-open-filter",
        label: "Open sidebar filters",
        group: "Sidebar",
        keywords: ["filter", "sort", "group", "view", "sidebar"],
        action: () => onOpenChange(true),
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
  }, [onOpenChange, views, setActiveView]);
  useRegisterCommands(commands);

  return (
    <div
      data-testid="sidebar-filter-bar"
      // Transparent so the bar inherits whatever surface hosts it — bg-card in
      // the dockview sidebar, bg-background in the mobile sheet — instead of
      // painting a clashing strip. Mobile px-2 leaves room for fixed touch
      // actions; md:px-3 aligns with the 12px content inset below.
      className="flex h-11 shrink-0 items-center gap-1 border-b border-border/60 bg-transparent px-2 md:h-[30px] md:px-3"
    >
      <SidebarViewChips />
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className={cn(
          "h-10 shrink-0 cursor-pointer px-2 text-[11px] md:h-6",
          newViewDisabledReason && "cursor-not-allowed opacity-45",
        )}
        onClick={() => {
          if (newViewDisabledReason) {
            toast.info(newViewDisabledReason);
            return;
          }
          if (!startNewView({ openPopover: false })) return;
          // Radix dismisses a controlled popover opened by this outside click.
          // Open through its registered trigger so focus and layering stay correct.
          filterTriggerRef.current?.focus();
          filterTriggerRef.current?.click();
        }}
        aria-disabled={!!newViewDisabledReason}
        data-testid="sidebar-new-view"
        data-disabled-reason={newViewDisabledReason ?? undefined}
        aria-label={
          newViewDisabledReason ? `New view unavailable. ${newViewDisabledReason}` : "New view"
        }
        title={newViewDisabledReason ?? undefined}
      >
        <IconPlus className="h-3.5 w-3.5" />
        New view
      </Button>
      <SidebarFilterPopover
        open={open}
        onOpenChange={onOpenChange}
        renameRequestedViewId={renameRequestedViewId}
        onRenameRequestHandled={consumeRenameRequest}
        trigger={
          <Button
            ref={filterTriggerRef}
            type="button"
            variant="ghost"
            size="icon"
            className="h-10 w-10 shrink-0 cursor-pointer md:h-6 md:w-6"
            data-testid="sidebar-filter-gear"
            aria-label="Sidebar filters"
          >
            <IconAdjustments className="h-4 w-4" />
            {hasDraft && (
              <span
                className="absolute right-2 top-2 h-1.5 w-1.5 rounded-full bg-amber-500 md:right-1 md:top-1"
                data-testid="sidebar-filter-gear-indicator"
              />
            )}
          </Button>
        }
      />
    </div>
  );
}
