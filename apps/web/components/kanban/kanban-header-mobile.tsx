"use client";

import { Button } from "@kandev/ui/button";
import { IconMenu2, IconSearch } from "@tabler/icons-react";
import { PageTopbar } from "@/components/page-topbar";
import { TopbarMetrics } from "@/components/system-metrics/topbar-metrics";
import { MobileMenuSheet } from "./mobile-menu-sheet";
import { useAppStore } from "@/components/state-provider";

type KanbanHeaderMobileProps = {
  workspaceId?: string;
  currentPage?: "kanban" | "tasks";
  hideTitle?: boolean;
  title: string;
  workspaceLabel: string;
  searchQuery?: string;
  onSearchChange?: (query: string) => void;
  isSearchLoading?: boolean;
  showHealthIndicator: boolean;
  onOpenHealthDialog: () => void;
};

export function KanbanHeaderMobile({
  workspaceId,
  currentPage = "kanban",
  hideTitle = false,
  title,
  workspaceLabel,
  searchQuery = "",
  onSearchChange,
  isSearchLoading = false,
  showHealthIndicator,
  onOpenHealthDialog,
}: KanbanHeaderMobileProps) {
  const isMenuOpen = useAppStore((state) => state.mobileKanban.isMenuOpen);
  const setMenuOpen = useAppStore((state) => state.setMobileKanbanMenuOpen);
  const isSearchOpen = useAppStore((state) => state.mobileKanban.isSearchOpen);
  const setSearchOpen = useAppStore((state) => state.setMobileKanbanSearchOpen);
  const isHome = title === "Home";

  const toggleSearch = () => {
    const next = !isSearchOpen;
    setSearchOpen(next);
    // Clear any active query when collapsing so results aren't filtered by a hidden search.
    if (!next) onSearchChange?.("");
  };

  return (
    <>
      {/* Keep mobile root chrome compact so metrics and actions stay visible. */}
      <PageTopbar
        title={title}
        backLabel={hideTitle ? "" : "Kandev"}
        className="h-10 px-3 py-1"
        variant="root"
        leftActions={
          hideTitle ? null : (
            <span className="flex min-w-0 max-w-[38vw] flex-col leading-tight">
              <span className="truncate text-sm font-medium text-muted-foreground">{title}</span>
              {!isHome && (
                <span className="truncate text-[10px] text-muted-foreground/60">
                  {workspaceLabel}
                </span>
              )}
            </span>
          )
        }
        actionsClassName="gap-2"
        actions={
          <>
            <TopbarMetrics size="lg" />
            {onSearchChange && (
              <Button
                variant={isSearchOpen ? "secondary" : "outline"}
                size="icon-lg"
                onClick={toggleSearch}
                className="cursor-pointer"
                aria-pressed={isSearchOpen}
                aria-label="Search tasks"
                data-testid="mobile-search-toggle"
              >
                <IconSearch className="h-4 w-4" />
              </Button>
            )}
            <Button
              variant="outline"
              size="icon-lg"
              onClick={() => setMenuOpen(true)}
              className="cursor-pointer"
            >
              <IconMenu2 className="h-4 w-4" />
              <span className="sr-only">Open menu</span>
            </Button>
          </>
        }
      />
      <MobileMenuSheet
        open={isMenuOpen}
        onOpenChange={setMenuOpen}
        workspaceId={workspaceId}
        currentPage={currentPage}
        searchQuery={searchQuery}
        onSearchChange={onSearchChange}
        isSearchLoading={isSearchLoading}
        showHealthIndicator={showHealthIndicator}
        onOpenHealthDialog={onOpenHealthDialog}
      />
    </>
  );
}
