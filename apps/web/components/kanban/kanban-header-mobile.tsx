"use client";

import { useRef, type MouseEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { PageTopbar } from "@/components/page-topbar";
import { MobileMenuSheet } from "./mobile-menu-sheet";
import { MobileListingContext } from "./mobile-listing-context";
import { MobileListingMenuButton } from "./mobile-listing-menu-button";
import { MobileListingMenuActions } from "./mobile-listing-menu-actions";
import type { TasksListDisplayOptions } from "./mobile-menu-task-list-options";
import { useAppStore } from "@/components/state-provider";
import type { TaskListingPage } from "@/lib/task-listing/view-navigation";

type KanbanHeaderMobileProps = {
  workspaceId?: string;
  currentPage?: TaskListingPage;
  title: string;
  workspaceLabel: string;
  searchQuery?: string;
  onSearchChange?: (query: string) => void;
  isSearchLoading?: boolean;
  tasksListOptions?: TasksListDisplayOptions;
  taskListingControls?: ReactNode;
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
  isSearchLoading = false,
  tasksListOptions,
  taskListingControls,
}: KanbanHeaderMobileProps) {
  const { t } = useTranslation();
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

  function closeMenuForAction() {
    restoreFocusRef.current = false;
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
    if (!next) onSearchChange?.("");
  }

  return (
    <>
      <PageTopbar
        title={title}
        testId={currentPage === "threads" ? "threads-mobile-topbar" : undefined}
        titleSlot={
          currentPage === "threads" ? (
            taskListingControls
          ) : (
            <MobileListingContext
              context={workspaceLabel}
              label={t(MODE_LABELS[currentPage])}
              onClick={openMenu}
              aria-haspopup="dialog"
              aria-expanded={isMenuOpen}
              data-testid="mobile-topbar-page-context"
            />
          )
        }
        className="h-14 min-h-14"
        showStatusTrigger={false}
        homeAffordance="none"
        freeWidth="lead"
        actions={
          <MobileListingMenuButton workspaceId={workspaceId} open={isMenuOpen} onClick={openMenu} />
        }
      />
      <MobileMenuSheet
        open={isMenuOpen}
        onOpenChange={setMenuOpen}
        onCloseAutoFocus={restoreMenuFocus}
        workspaceId={workspaceId}
        currentPage={currentPage}
        searchQuery={searchQuery}
        onSearchChange={onSearchChange}
        isSearchLoading={isSearchLoading}
        tasksListOptions={tasksListOptions}
        pageActions={
          <MobileListingMenuActions
            workspaceId={workspaceId}
            workspaceLabel={workspaceLabel}
            currentPage={currentPage}
            open={isMenuOpen}
            closeMenu={closeMenuForAction}
            onToggleSearch={onSearchChange ? toggleSearch : undefined}
            isSearchOpen={isSearchOpen}
            returnFocusRef={openerRef}
          />
        }
      />
    </>
  );
}
