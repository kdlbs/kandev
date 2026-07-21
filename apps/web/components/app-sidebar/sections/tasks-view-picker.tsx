"use client";

import { useMemo, useRef } from "react";
import { IconChevronDown, IconAdjustments, IconCheck, IconPlus } from "@tabler/icons-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { useAppStore } from "@/components/state-provider";
import { SidebarFilterPopover } from "@/components/task/sidebar-filter/sidebar-filter-popover";
import { useSidebarViewPopover } from "@/components/task/sidebar-filter/use-sidebar-view-popover";
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
  const openPopoverAfterPickerCloseRef = useRef(false);
  const {
    open,
    onOpenChange,
    startNewView,
    renameRequestedViewId,
    consumeRenameRequest,
    newViewDisabledReason,
  } = useSidebarViewPopover();

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
        <DropdownMenuContent
          align="end"
          className="w-52"
          onCloseAutoFocus={(event) => {
            if (!openPopoverAfterPickerCloseRef.current) return;
            event.preventDefault();
            openPopoverAfterPickerCloseRef.current = false;
            onOpenChange(true);
          }}
        >
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
          <DropdownMenuSeparator />
          <NewViewMenuItem
            disabledReason={newViewDisabledReason}
            hasDraft={draft !== null}
            onSelect={() => {
              openPopoverAfterPickerCloseRef.current = startNewView({ openPopover: false });
            }}
          />
        </DropdownMenuContent>
      </DropdownMenu>
      <SidebarFilterPopover
        open={open}
        onOpenChange={onOpenChange}
        renameRequestedViewId={renameRequestedViewId}
        onRenameRequestHandled={consumeRenameRequest}
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

function NewViewMenuItem({
  disabledReason,
  hasDraft,
  onSelect,
}: {
  disabledReason: string | null;
  hasDraft: boolean;
  onSelect: () => void;
}) {
  return (
    <DropdownMenuItem
      disabled={!!disabledReason}
      onSelect={onSelect}
      data-testid="sidebar-new-view"
      aria-label={disabledReason ? `New view unavailable. ${disabledReason}` : "New view"}
      title={disabledReason ?? undefined}
      className="cursor-pointer gap-2"
    >
      <IconPlus className="h-3.5 w-3.5" />
      <span>New view</span>
      {disabledReason && (
        <span className="ml-auto text-[10px] text-muted-foreground" aria-hidden="true">
          {hasDraft ? "Save/discard first" : "50 max"}
        </span>
      )}
    </DropdownMenuItem>
  );
}
