"use client";

import { useRef, type MouseEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconAlertTriangle } from "@tabler/icons-react";
import { IconSearch } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { PageTopbar } from "@/components/page-topbar";
import { MobileMenuSheet } from "./mobile-menu-sheet";
import { MobileListingContext } from "./mobile-listing-context";
import { AppNavSheet } from "@/components/navigation/app-nav-sheet";
import { MobileListingMenuActions } from "./mobile-listing-menu-actions";
import type { TasksListDisplayOptions } from "./mobile-menu-task-list-options";
import { useAppStore } from "@/components/state-provider";
import type { TaskListingPage } from "@/lib/task-listing/view-navigation";
import type { MobileListFacetSettings } from "@/lib/tasks/mobile-list-query";

type KanbanHeaderMobileProps = {
  workspaceId?: string;
  currentPage?: TaskListingPage;
  title: string;
  workspaceLabel: string;
  searchQuery?: string;
  onSearchChange?: (query: string) => void;
  onSearchClose?: () => void;
  isSearchLoading?: boolean;
  tasksListOptions?: TasksListDisplayOptions;
  taskListingControls?: ReactNode;
  mobileListingStatus?: ReactNode;
  mobileListFacets?: MobileListFacetSettings;
};

const MODE_LABELS: Record<TaskListingPage, string> = {
  kanban: "kanban:kanban",
  tasks: "kanban:list",
  threads: "threads:title",
};

export function KanbanHeaderMobile({
  workspaceId,
  currentPage = "kanban",
  title,
  workspaceLabel,
  searchQuery = "",
  onSearchChange,
  onSearchClose,
  isSearchLoading = false,
  tasksListOptions,
  taskListingControls,
  mobileListingStatus,
  mobileListFacets,
}: KanbanHeaderMobileProps) {
  const isMenuOpen = useAppStore((state) => state.mobileKanban.isMenuOpen);
  const setMenuOpen = useAppStore((state) => state.setMobileKanbanMenuOpen);
  const isSearchOpen = useAppStore((state) => state.mobileKanban.isSearchOpen);
  const setSearchOpen = useAppStore((state) => state.setMobileKanbanSearchOpen);
  const openerRef = useRef<HTMLButtonElement | null>(null);
  const restoreFocusRef = useRef(true);

  function openMenu(event: MouseEvent<HTMLButtonElement>) {
    openerRef.current = event.currentTarget;
    restoreFocusRef.current = true;
    setMenuOpen(true);
  }

  function closeMenuForAction(restoreFocus = false) {
    restoreFocusRef.current = restoreFocus;
    setMenuOpen(false);
  }

  function restoreMenuFocus(event: Event) {
    event.preventDefault();
    if (restoreFocusRef.current && openerRef.current?.isConnected) {
      openerRef.current.focus({ preventScroll: true });
    }
  }

  function toggleSearch() {
    const next = !isSearchOpen;
    setSearchOpen(next);
    // A hidden search must not leave the listing filtered.
    if (!next) {
      if (onSearchClose) onSearchClose();
      else onSearchChange?.("");
    }
  }

  return (
    <>
      <MobileListingTopbar
        currentPage={currentPage}
        title={title}
        workspaceLabel={workspaceLabel}
        mobileListingStatus={mobileListingStatus}
        isMenuOpen={isMenuOpen}
        isSearchOpen={isSearchOpen}
        hasSearch={currentPage === "tasks" && !!onSearchChange}
        onOpenMenu={openMenu}
        onToggleSearch={toggleSearch}
      />
      <MobileMenuSheet
        listingOnly
        open={isMenuOpen}
        onOpenChange={setMenuOpen}
        onCloseAutoFocus={restoreMenuFocus}
        workspaceId={workspaceId}
        currentPage={currentPage}
        searchQuery={searchQuery}
        onSearchChange={onSearchChange}
        isSearchLoading={isSearchLoading}
        tasksListOptions={tasksListOptions}
        listingControls={taskListingControls}
        mobileListFacets={mobileListFacets}
        pageActions={
          currentPage === "tasks" ? null : (
            <MobileListingMenuActions
              showWorkspaceActions={false}
              workspaceId={workspaceId}
              workspaceLabel={workspaceLabel}
              currentPage={currentPage}
              open={isMenuOpen}
              closeMenu={closeMenuForAction}
              onToggleSearch={onSearchChange ? toggleSearch : undefined}
              isSearchOpen={isSearchOpen}
              returnFocusRef={openerRef}
            />
          )
        }
      />
    </>
  );
}

function MobileListingTopbar({
  currentPage,
  title,
  workspaceLabel,
  mobileListingStatus,
  isMenuOpen,
  isSearchOpen,
  hasSearch,
  onOpenMenu,
  onToggleSearch,
}: {
  currentPage: TaskListingPage;
  title: string;
  workspaceLabel: string;
  mobileListingStatus?: ReactNode;
  isMenuOpen: boolean;
  isSearchOpen: boolean;
  hasSearch: boolean;
  onOpenMenu: (event: MouseEvent<HTMLButtonElement>) => void;
  onToggleSearch: () => void;
}) {
  const { t } = useTranslation();
  return (
    <PageTopbar
      title={title}
      testId={currentPage === "threads" ? "threads-mobile-topbar" : undefined}
      titleSlot={
        <div className="flex min-w-0 items-center">
          <MobileListingContext
            context={workspaceLabel}
            label={t(MODE_LABELS[currentPage])}
            status={currentPage === "threads" ? <ThreadViewSyncStatus /> : undefined}
            onClick={onOpenMenu}
            aria-haspopup="dialog"
            aria-expanded={isMenuOpen}
            data-testid="mobile-topbar-page-context"
          />
          {mobileListingStatus}
        </div>
      }
      className="h-14 min-h-14"
      showStatusTrigger={false}
      homeAffordance="none"
      freeWidth="lead"
      actions={
        <div className="flex items-center gap-1">
          {hasSearch && (
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="size-10 cursor-pointer"
              aria-label={t("kanban:searchTasks")}
              aria-expanded={isSearchOpen}
              aria-controls="mobile-list-search-row"
              onClick={onToggleSearch}
              data-testid="mobile-list-search-toggle"
            >
              <IconSearch className="size-4" aria-hidden="true" />
            </Button>
          )}
          <AppNavSheet />
        </div>
      }
    />
  );
}

function ThreadViewSyncStatus() {
  const { t } = useTranslation();
  const error = useAppStore((state) => state.threadViews.syncError);
  if (!error) return null;
  return (
    <span
      role="status"
      className="shrink-0 text-destructive"
      data-testid="threads-mobile-view-sync-status"
    >
      <IconAlertTriangle aria-hidden="true" className="size-4" />
      <span className="sr-only">{t("threads:failedToSyncViews")}</span>
    </span>
  );
}
