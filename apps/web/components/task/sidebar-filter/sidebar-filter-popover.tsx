"use client";

import { useRef, type ComponentProps, type RefObject } from "react";
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
import { isActionConfirmationTarget } from "@/components/confirmation/action-confirm-popover";
import { SavedTaskViewDeleteConfirmation } from "@/components/confirmation/saved-task-view-delete-confirmation";
import {
  useSavedTaskViewDeleteConfirmation,
  type SavedTaskViewDeleteTarget,
} from "@/components/confirmation/use-saved-task-view-delete-confirmation";
import { sidebarViewName } from "@/lib/state/slices/ui/sidebar-view-builtins";

type Props = {
  trigger: React.ReactNode;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  renameRequestedViewId?: string | null;
  onRenameRequestHandled?: (viewId: string) => void;
};

type SidebarFilterSurfaceProps = Pick<Props, "trigger" | "open" | "onOpenChange"> & {
  editor: React.ReactNode;
  deletion: {
    target: SavedTaskViewDeleteTarget | null;
    anchorRef: RefObject<HTMLButtonElement | null>;
    close: () => void;
  };
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
  const { usesDesktopWorkbench, isFinePointer } = useResponsiveBreakpoint();
  const deletion = useSavedTaskViewDeleteConfirmation<HTMLButtonElement>(views);
  const popoverContentRef = useRef<HTMLDivElement>(null);
  const activeView: SidebarView | undefined = views.find((view) => view.id === activeViewId);
  const current = createSidebarViewEditorCurrent(activeView, storedDraft);
  const hasDraft = !!storedDraft && activeView?.id === storedDraft.baseViewId;
  const usesInlineDeleteConfirmation = !usesDesktopWorkbench || !isFinePointer;

  const inlineDeleteConfirmation =
    usesInlineDeleteConfirmation && deletion.target ? (
      <SavedTaskViewDeleteConfirmation
        target={deletion.target}
        presentation="inline"
        open
        anchorRef={deletion.anchorRef}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) deletion.close();
        }}
        onConfirm={deleteView}
      />
    ) : undefined;
  const headerProps: ComponentProps<typeof ViewHeaderRow> = {
    activeView,
    hasDraft,
    canDelete: views.length > 1,
    onSaveOverwrite: saveOverwrite,
    onSaveAs: saveAs,
    onRename: renameView,
    onDiscard: discard,
    onDelete: () => {
      if (!activeView) return;
      deletion.request({ id: activeView.id, label: sidebarViewName(activeView, t) });
    },
    deleteAnchorRef: deletion.anchorRef,
    deleteDensity: usesInlineDeleteConfirmation ? "touch" : "compact",
    deleteConfirmation: inlineDeleteConfirmation,
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
      <DesktopSidebarFilterSurface
        trigger={trigger}
        open={open}
        onOpenChange={onOpenChange}
        editor={editor}
        deletion={deletion}
        contentRef={popoverContentRef}
        isFinePointer={isFinePointer}
        onDelete={deleteView}
      />
    );
  }

  return (
    <MobileSidebarFilterSurface
      trigger={trigger}
      open={open}
      onOpenChange={onOpenChange}
      editor={editor}
      deletion={deletion}
      title={t("task:sidebarFilters")}
    />
  );
}

function DesktopSidebarFilterSurface({
  trigger,
  open,
  onOpenChange,
  editor,
  deletion,
  contentRef,
  isFinePointer,
  onDelete,
}: SidebarFilterSurfaceProps & {
  contentRef: RefObject<HTMLDivElement | null>;
  isFinePointer: boolean;
  onDelete: (id: string) => void;
}) {
  return (
    <>
      <Popover
        open={open}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) deletion.close();
          onOpenChange(nextOpen);
        }}
      >
        <PopoverTrigger asChild>{trigger}</PopoverTrigger>
        <PopoverContent
          ref={contentRef}
          className="max-h-[var(--radix-popper-available-height)] w-[calc(100vw-1rem)] max-w-[22rem] overflow-y-auto gap-0 p-0"
          style={{ maxHeight: "var(--radix-popper-available-height, calc(100dvh - 1rem))" }}
          align="end"
          data-testid="sidebar-filter-popover"
          onFocusOutside={(event) => {
            if (deletion.target && isActionConfirmationTarget(event.target)) event.preventDefault();
          }}
          onInteractOutside={(event) => {
            if (deletion.target && isActionConfirmationTarget(event.target)) event.preventDefault();
          }}
        >
          {editor}
        </PopoverContent>
      </Popover>
      {isFinePointer && deletion.target ? (
        <SavedTaskViewDeleteConfirmation
          target={deletion.target}
          presentation="popover"
          open
          anchorRef={deletion.anchorRef}
          focusBoundaryRef={contentRef}
          onOpenChange={(nextOpen) => {
            if (!nextOpen) deletion.close();
          }}
          onConfirm={onDelete}
        />
      ) : null}
    </>
  );
}

function MobileSidebarFilterSurface({
  trigger,
  open,
  onOpenChange,
  editor,
  deletion,
  title,
}: SidebarFilterSurfaceProps & { title: string }) {
  return (
    <Drawer
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen) deletion.close();
        onOpenChange(nextOpen);
      }}
    >
      <DrawerTrigger asChild>{trigger}</DrawerTrigger>
      <DrawerContent
        data-testid="sidebar-filter-drawer"
        className="h-[min(90dvh,48rem)] max-h-[calc(100dvh-1rem)] overflow-hidden rounded-t-xl"
      >
        <DrawerHeader className="shrink-0 border-b px-4 pb-3 pt-5 text-left">
          <DrawerTitle>{title}</DrawerTitle>
        </DrawerHeader>
        <div
          data-testid="sidebar-filter-popover"
          className="min-h-0 flex-1 overflow-y-auto overscroll-contain pb-[calc(1rem+env(safe-area-inset-bottom))]"
        >
          {editor}
        </div>
      </DrawerContent>
    </Drawer>
  );
}
