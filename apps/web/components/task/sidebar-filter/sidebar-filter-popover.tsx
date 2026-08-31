"use client";

import type { ComponentProps } from "react";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle, DrawerTrigger } from "@kandev/ui/drawer";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { useAppStore } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { FilterClause, SidebarView } from "@/lib/state/slices/ui/sidebar-view-types";
import {
  SidebarViewEditor,
  createSidebarFilterClause,
  createSidebarViewEditorCurrent,
} from "./sidebar-view-editor";
import { ViewHeaderRow } from "./view-manager";
import { useTranslation } from "react-i18next";

type Props = {
  trigger: React.ReactNode;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  renameRequestedViewId?: string | null;
  onRenameRequestHandled?: (viewId: string) => void;
};

export function SidebarFilterPopover({
  trigger,
  open,
  onOpenChange,
  renameRequestedViewId,
  onRenameRequestHandled,
}: Props) {
  const { t } = useTranslation();
  const views = useAppStore((s) => s.sidebarViews.views);
  const activeViewId = useAppStore((s) => s.sidebarViews.activeViewId);
  const storedDraft = useAppStore((s) => s.sidebarViews.draft);
  const updateDraft = useAppStore((s) => s.updateSidebarDraft);
  const saveAs = useAppStore((s) => s.saveSidebarDraftAs);
  const saveOverwrite = useAppStore((s) => s.saveSidebarDraftOverwrite);
  const discard = useAppStore((s) => s.discardSidebarDraft);
  const deleteView = useAppStore((s) => s.deleteSidebarView);
  const renameView = useAppStore((s) => s.renameSidebarView);
  const { usesDesktopWorkbench } = useResponsiveBreakpoint();
  const activeView: SidebarView | undefined = views.find((view) => view.id === activeViewId);
  const current = createSidebarViewEditorCurrent(activeView, storedDraft);
  const hasDraft = !!storedDraft && activeView?.id === storedDraft.baseViewId;

  const headerProps: ComponentProps<typeof ViewHeaderRow> = {
    activeView,
    hasDraft,
    canDelete: views.length > 1,
    onSaveOverwrite: saveOverwrite,
    onSaveAs: saveAs,
    onRename: renameView,
    onDiscard: discard,
    onDelete: () => activeView && deleteView(activeView.id),
    renameRequestedViewId,
    onRenameRequestHandled,
  };
  const editor = (
    <SidebarViewEditor
      current={current}
      isDrawerLayout={!usesDesktopWorkbench}
      headerProps={headerProps}
      onUpdate={updateDraft}
      onAddFilter={() =>
        updateDraft({ filters: [...current.filters, createSidebarFilterClause()] })
      }
      onChangeClause={(next: FilterClause) =>
        updateDraft({
          filters: current.filters.map((clause) => (clause.id === next.id ? next : clause)),
        })
      }
      onRemoveClause={(id) =>
        updateDraft({ filters: current.filters.filter((clause) => clause.id !== id) })
      }
    />
  );

  if (usesDesktopWorkbench) {
    return (
      <Popover open={open} onOpenChange={onOpenChange}>
        <PopoverTrigger asChild>{trigger}</PopoverTrigger>
        <PopoverContent
          className="max-h-[var(--radix-popper-available-height)] w-[calc(100vw-1rem)] max-w-[22rem] overflow-y-auto p-0"
          align="end"
          data-testid="sidebar-filter-popover"
        >
          {editor}
        </PopoverContent>
      </Popover>
    );
  }

  return (
    <Drawer open={open} onOpenChange={onOpenChange}>
      <DrawerTrigger asChild>{trigger}</DrawerTrigger>
      <DrawerContent
        data-testid="sidebar-filter-drawer"
        className="h-[min(90dvh,48rem)] max-h-[calc(100dvh-1rem)] overflow-hidden rounded-t-xl"
      >
        <DrawerHeader className="shrink-0 border-b px-4 pb-3 pt-5 text-left">
          <DrawerTitle>{t("task:sidebarFilters")}</DrawerTitle>
        </DrawerHeader>
        <div
          data-testid="sidebar-filter-popover"
          className="min-h-0 flex-1 overflow-y-auto pb-[calc(1rem+env(safe-area-inset-bottom))]"
        >
          {editor}
        </div>
      </DrawerContent>
    </Drawer>
  );
}
