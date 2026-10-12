"use client";

import { selectSidebarViews } from "@/lib/state/slices/ui/sidebar-workspace-state";

import { useMemo, useRef } from "react";
import { IconFilter, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { toast } from "@/lib/toast/sonner";
import { useAppStore } from "@/components/state-provider";
import { useRegisterCommands } from "@/hooks/use-register-commands";
import type { CommandItem } from "@/lib/commands/types";
import type { SidebarView } from "@/lib/state/slices/ui/sidebar-view-types";
import { cn } from "@/lib/utils";
import { SidebarViewChips } from "./sidebar-view-chips";
import { SidebarFilterIndicators } from "@/components/task/sidebar-filter/sidebar-filter-indicators";
import { SidebarFilterPopover } from "./sidebar-filter-popover";
import { useSidebarViewPopover } from "./use-sidebar-view-popover";
import { useTranslation } from "react-i18next";
import { sidebarViewName } from "@/lib/state/slices/ui/sidebar-view-builtins";

// `group` is the command-palette bucket shared with `components/session-commands.tsx`;
// it is grouping taxonomy rather than this surface's copy, so it migrates with
// that file. `keywords` are search tokens, not displayed copy.
function useSidebarCommands(
  views: SidebarView[],
  onOpenChange: (open: boolean) => void,
  setActiveView: (id: string) => void,
): CommandItem[] {
  const { t } = useTranslation();
  return useMemo<CommandItem[]>(() => {
    const list: CommandItem[] = [
      {
        id: "sidebar-open-filter",
        label: t("task:openSidebarFilters"),
        group: "Sidebar",
        keywords: ["filter", "sort", "group", "view", "sidebar"],
        action: () => onOpenChange(true),
      },
    ];
    for (const view of views) {
      list.push({
        id: `sidebar-switch-view-${view.id}`,
        label: t("task:switchSidebarView", { name: sidebarViewName(view, t) }),
        group: "Sidebar",
        keywords: ["view", "switch", "sidebar", sidebarViewName(view, t).toLowerCase()],
        action: () => setActiveView(view.id),
      });
    }
    return list;
  }, [t, onOpenChange, views, setActiveView]);
}

export function SidebarFilterBar() {
  const { t } = useTranslation();
  const filterTriggerRef = useRef<HTMLButtonElement>(null);
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const views = useAppStore((s) => selectSidebarViews(s).views);
  const setActiveView = useAppStore((s) => s.setSidebarActiveView);
  const {
    open,
    onOpenChange,
    startNewView,
    renameRequestedViewId,
    consumeRenameRequest,
    newViewDisabledReason,
  } = useSidebarViewPopover();

  useRegisterCommands(useSidebarCommands(views, onOpenChange, setActiveView));

  if (!workspaceId) return null;

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
          "h-11 shrink-0 cursor-pointer px-2 text-[11px] md:h-7 [@media(pointer:coarse)]:min-h-11",
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
          newViewDisabledReason
            ? t("task:newViewUnavailable", { newViewDisabledReason })
            : t("task:newView")
        }
        title={newViewDisabledReason ?? undefined}
      >
        <IconPlus className="h-3.5 w-3.5" />
        {t("task:newView")}
      </Button>
      <SidebarFilterPopover
        key={workspaceId}
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
            className="relative h-11 w-11 shrink-0 cursor-pointer md:h-7 md:w-7 [@media(pointer:coarse)]:size-11"
            data-testid="sidebar-filter-gear"
            aria-label={t("task:sidebarFilters")}
          >
            <IconFilter className="h-4 w-4" />
            <SidebarFilterIndicators />
          </Button>
        }
      />
    </div>
  );
}
