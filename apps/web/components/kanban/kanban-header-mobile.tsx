"use client";

import { Button } from "@kandev/ui/button";
import { IconMenu2, IconMessageCircle, IconSearch } from "@tabler/icons-react";
import Link from "@/components/routing/app-link";
import { PageTopbar } from "@/components/page-topbar";
import { TopbarMetrics } from "@/components/system-metrics/topbar-metrics";
import { MainTopBarPluginActions } from "./main-top-bar-plugin-actions";
import { MobileMenuSheet } from "./mobile-menu-sheet";
import type { TasksListDisplayOptions } from "./mobile-menu-task-list-options";
import { useAppStore } from "@/components/state-provider";
import { useAppStatusDrawer } from "@/components/app-status-bar/app-status-surface-provider";
import { useConnectionIssueCopy } from "@/components/app-status-bar/connection-status-item";
import { useQuickChatLauncher } from "@/hooks/use-quick-chat-launcher";
import { workspaceHomeHref } from "@/components/app-sidebar/app-sidebar-workspace-navigation";
import { cn } from "@/lib/utils";

type KanbanHeaderMobileProps = {
  workspaceId?: string;
  currentPage?: "kanban" | "tasks";
  hideTitle?: boolean;
  title: string;
  workspaceLabel: string;
  searchQuery?: string;
  onSearchChange?: (query: string) => void;
  isSearchLoading?: boolean;
  tasksListOptions?: TasksListDisplayOptions;
  showHealthIndicator: boolean;
  onOpenHealthDialog: () => void;
};

function MobileBrandLink({ workspaceId }: Pick<KanbanHeaderMobileProps, "workspaceId">) {
  return (
    <Link
      href={workspaceHomeHref(workspaceId ? { id: workspaceId } : undefined)}
      aria-label="Kandev home"
      className="relative z-10 shrink-0 cursor-pointer text-[15px] font-semibold leading-none transition-colors hover:text-foreground/80"
    >
      Kandev
    </Link>
  );
}

function MobileHeaderActions({
  workspaceId,
  workspaceLabel,
  currentPage,
  onSearchChange,
  isSearchOpen,
  handleOpenQuickChat,
  toggleSearch,
  setMenuOpen,
}: {
  workspaceId?: string;
  workspaceLabel: string;
  currentPage: "kanban" | "tasks";
  onSearchChange?: (query: string) => void;
  isSearchOpen: boolean;
  handleOpenQuickChat: () => void;
  toggleSearch: () => void;
  setMenuOpen: (open: boolean) => void;
}) {
  const { issueSeverity } = useAppStatusDrawer();
  const issueDetails = useConnectionIssueCopy(issueSeverity);

  return (
    <>
      <MainTopBarPluginActions
        workspaceId={workspaceId}
        workspaceLabel={workspaceLabel}
        currentPage={currentPage}
      />
      <TopbarMetrics size="lg" />
      {workspaceId && (
        <Button
          variant="outline"
          size="icon-lg"
          onClick={handleOpenQuickChat}
          className="cursor-pointer"
          aria-label="Quick Chat"
          data-testid="mobile-quick-chat-button"
        >
          <IconMessageCircle className="h-4 w-4" />
        </Button>
      )}
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
        className={cn(
          "relative cursor-pointer",
          issueSeverity === "lost" && "text-destructive",
          issueSeverity === "unstable" && "text-amber-500",
        )}
        aria-label={issueDetails ? `${issueDetails.description} Open menu` : "Open menu"}
        data-connection-severity={issueSeverity === "none" ? undefined : issueSeverity}
      >
        <IconMenu2 className="h-4 w-4" />
        {issueDetails && (
          <span
            className={cn(
              "absolute right-1.5 top-1.5 size-2 rounded-full ring-2 ring-background",
              issueDetails.dotClass,
            )}
            aria-hidden="true"
          />
        )}
      </Button>
    </>
  );
}

export function KanbanHeaderMobile({
  workspaceId,
  currentPage = "kanban",
  hideTitle = false,
  title,
  workspaceLabel,
  searchQuery = "",
  onSearchChange,
  isSearchLoading = false,
  tasksListOptions,
  showHealthIndicator,
  onOpenHealthDialog,
}: KanbanHeaderMobileProps) {
  const isMenuOpen = useAppStore((state) => state.mobileKanban.isMenuOpen);
  const setMenuOpen = useAppStore((state) => state.setMobileKanbanMenuOpen);
  const isSearchOpen = useAppStore((state) => state.mobileKanban.isSearchOpen);
  const setSearchOpen = useAppStore((state) => state.setMobileKanbanSearchOpen);
  const handleOpenQuickChat = useQuickChatLauncher(workspaceId);
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
        backLabel=""
        leading={<MobileBrandLink workspaceId={workspaceId} />}
        showStatusTrigger={false}
        className="h-10 px-3 py-1"
        variant="root"
        leftActions={
          hideTitle || isHome ? null : (
            <span className="flex min-w-0 max-w-[38vw] flex-col leading-tight">
              <span className="truncate text-sm font-medium text-muted-foreground">{title}</span>
              <span className="truncate text-[10px] text-muted-foreground/60">
                {workspaceLabel}
              </span>
            </span>
          )
        }
        actionsClassName="gap-2"
        actions={
          <MobileHeaderActions
            workspaceId={workspaceId}
            workspaceLabel={workspaceLabel}
            currentPage={currentPage}
            onSearchChange={onSearchChange}
            isSearchOpen={isSearchOpen}
            handleOpenQuickChat={handleOpenQuickChat}
            toggleSearch={toggleSearch}
            setMenuOpen={setMenuOpen}
          />
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
        tasksListOptions={tasksListOptions}
        showHealthIndicator={showHealthIndicator}
        onOpenHealthDialog={onOpenHealthDialog}
      />
    </>
  );
}
