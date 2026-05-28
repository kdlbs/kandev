"use client";

import { Button } from "@kandev/ui/button";
import { IconMenu2 } from "@tabler/icons-react";
import { PageTopbar } from "@/components/page-topbar";
import { MobileMenuSheet } from "./mobile-menu-sheet";
import { useAppStore } from "@/components/state-provider";

type KanbanHeaderMobileProps = {
  workspaceId?: string;
  currentPage?: "kanban" | "tasks";
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

  return (
    <>
      <PageTopbar
        title={title}
        subtitle={workspaceLabel}
        backLabel={title === "Home" ? "" : "Kandev"}
        className="h-10 px-3 py-1"
        variant={title === "Home" ? "root" : "breadcrumb"}
        actions={
          <Button
            variant="outline"
            size="icon-lg"
            onClick={() => setMenuOpen(true)}
            className="cursor-pointer"
          >
            <IconMenu2 className="h-4 w-4" />
            <span className="sr-only">Open menu</span>
          </Button>
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
