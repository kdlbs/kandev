"use client";

import Link from "next/link";
import { Button } from "@kandev/ui/button";
import { IconMenu2 } from "@tabler/icons-react";
import { MobileMenuSheet } from "./mobile-menu-sheet";
import { useAppStore } from "@/components/state-provider";

type KanbanHeaderMobileProps = {
  workspaceId?: string;
  currentPage?: "kanban" | "tasks";
  searchQuery?: string;
  onSearchChange?: (query: string) => void;
  isSearchLoading?: boolean;
};

export function KanbanHeaderMobile({
  workspaceId,
  currentPage = "kanban",
  searchQuery = "",
  onSearchChange,
  isSearchLoading = false,
}: KanbanHeaderMobileProps) {
  const isMenuOpen = useAppStore((state) => state.mobileKanban.isMenuOpen);
  const setMenuOpen = useAppStore((state) => state.setMobileKanbanMenuOpen);

  return (
    <>
      <header className="flex items-center justify-between p-4 pb-3">
        <Link href="/" className="text-xl font-bold hover:opacity-80">
          KanDev
        </Link>
        <Button
          variant="outline"
          size="icon"
          onClick={() => setMenuOpen(true)}
          className="cursor-pointer h-10 w-10"
        >
          <IconMenu2 className="h-5 w-5" />
          <span className="sr-only">Open menu</span>
        </Button>
      </header>
      <MobileMenuSheet
        open={isMenuOpen}
        onOpenChange={setMenuOpen}
        workspaceId={workspaceId}
        currentPage={currentPage}
        searchQuery={searchQuery}
        onSearchChange={onSearchChange}
        isSearchLoading={isSearchLoading}
      />
    </>
  );
}
