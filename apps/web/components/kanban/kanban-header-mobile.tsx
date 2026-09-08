"use client";

import { Button } from "@kandev/ui/button";
import { IconMenu2, IconMessageCircle, IconSearch, IconTerminal2 } from "@tabler/icons-react";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { PageTopbar } from "@/components/page-topbar";
import { TopbarMetrics } from "@/components/system-metrics/topbar-metrics";
import { MainTopBarPluginActions } from "./main-top-bar-plugin-actions";
import { MobileTopbarActionStrip } from "./mobile-topbar-action-strip";
import { MobileMenuSheet } from "./mobile-menu-sheet";
import type { TasksListDisplayOptions } from "./mobile-menu-task-list-options";
import { useAppStore } from "@/components/state-provider";
import { useAppStatusDrawer } from "@/components/app-status-bar/app-status-surface-provider";
import { useConnectionIssueCopy } from "@/components/app-status-bar/connection-status-item";
import { useQuickChatLauncher } from "@/hooks/use-quick-chat-launcher";
import { useQuickTerminalLauncher } from "@/hooks/use-quick-terminal-launcher";
import { workspaceHomeHref } from "@/lib/navigation/workspace-home";
import { QuickChatActivityIndicator } from "@/components/quick-chat/quick-chat-activity-indicator";
import { useQuickChatActivity } from "@/components/quick-chat/use-quick-chat-activity";
import { cn } from "@/lib/utils";
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

function MobileBrandLink({ workspaceId }: Pick<KanbanHeaderMobileProps, "workspaceId">) {
  const { t } = useTranslation();
  return (
    <Link
      href={workspaceHomeHref(workspaceId ? { id: workspaceId } : undefined)}
      aria-label={t("kanban:kandevHome")}
      className="relative z-10 shrink-0 cursor-pointer text-[15px] font-semibold leading-none transition-colors hover:text-foreground/80"
      data-testid="mobile-topbar-brand"
    >
      Kandev
    </Link>
  );
}

function MobileLauncherTarget({
  children,
  onClick,
  testId,
}: {
  children: ReactNode;
  onClick: () => void;
  testId: string;
}) {
  return (
    <span
      className="flex h-11 w-8 shrink-0 cursor-pointer items-center justify-center"
      data-testid={testId}
      onClick={onClick}
    >
      {children}
    </span>
  );
}

function MobileQuickChatButton({
  workspaceId,
  onClick,
}: {
  workspaceId: string;
  onClick: () => void;
}) {
  const { activity: quickChatActivity, label: quickChatLabel } = useQuickChatActivity(workspaceId);
  return (
    <MobileLauncherTarget onClick={onClick} testId="mobile-quick-chat-hit-target">
      <Button
        variant="outline"
        size="icon-lg"
        aria-label={quickChatLabel}
        data-testid="mobile-quick-chat-button"
      >
        <span className="relative flex">
          <IconMessageCircle className="h-4 w-4" />
          <QuickChatActivityIndicator activity={quickChatActivity} />
        </span>
      </Button>
    </MobileLauncherTarget>
  );
}

function MobileHeaderActionItems({
  workspaceId,
  workspaceLabel,
  currentPage,
  onSearchChange,
  isSearchOpen,
  handleOpenQuickChat,
  handleOpenQuickTerminal,
  toggleSearch,
  taskListingControls,
}: {
  workspaceId?: string;
  workspaceLabel: string;
  currentPage: TaskListingPage;
  onSearchChange?: (query: string) => void;
  isSearchOpen: boolean;
  handleOpenQuickChat: () => void;
  handleOpenQuickTerminal: () => void;
  toggleSearch: () => void;
  taskListingControls?: ReactNode;
}) {
  const { t } = useTranslation();

  return (
    <>
      {taskListingControls}
      <MainTopBarPluginActions
        workspaceId={workspaceId}
        workspaceLabel={workspaceLabel}
        currentPage={currentPage}
        presentation="mobile"
      />
      <TopbarMetrics size="lg" mobile />
      {workspaceId && (
        <MobileLauncherTarget
          onClick={handleOpenQuickTerminal}
          testId="mobile-quick-terminal-hit-target"
        >
          <Button
            variant="outline"
            size="icon-lg"
            aria-label={t("sidebar:quickTerminal")}
            data-testid="mobile-quick-terminal-button"
          >
            <IconTerminal2 className="h-4 w-4" />
          </Button>
        </MobileLauncherTarget>
      )}
      {workspaceId && (
        <MobileQuickChatButton workspaceId={workspaceId} onClick={handleOpenQuickChat} />
      )}
      {onSearchChange && (
        <Button
          variant={isSearchOpen ? "secondary" : "outline"}
          size="icon-lg"
          onClick={toggleSearch}
          className="cursor-pointer"
          aria-pressed={isSearchOpen}
          aria-label={t("kanban:searchTasks")}
          data-testid="mobile-search-toggle"
        >
          <IconSearch className="h-4 w-4" />
        </Button>
      )}
    </>
  );
}

function MobileHeaderActions(
  props: Parameters<typeof MobileHeaderActionItems>[0] & {
    setMenuOpen: (open: boolean) => void;
  },
) {
  const { setMenuOpen, ...actionProps } = props;
  const { t } = useTranslation();
  const { issueSeverity } = useAppStatusDrawer();
  const issueDetails = useConnectionIssueCopy(issueSeverity);

  return (
    <>
      <MobileTopbarActionStrip>
        <MobileHeaderActionItems {...actionProps} />
      </MobileTopbarActionStrip>
      <Button
        variant="outline"
        size="icon-lg"
        onClick={() => setMenuOpen(true)}
        className={cn(
          "relative cursor-pointer",
          issueSeverity === "lost" && "text-destructive",
          issueSeverity === "unstable" && "text-amber-500",
        )}
        aria-label={
          issueDetails
            ? t("kanban:openMenuWithStatus", { description: issueDetails.description })
            : t("kanban:openMenu")
        }
        data-connection-severity={issueSeverity === "none" ? undefined : issueSeverity}
        data-testid="mobile-topbar-menu"
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
  title,
  workspaceLabel,
  searchQuery = "",
  onSearchChange,
  isSearchLoading = false,
  tasksListOptions,
  taskListingControls,
}: KanbanHeaderMobileProps) {
  const isMenuOpen = useAppStore((state) => state.mobileKanban.isMenuOpen);
  const setMenuOpen = useAppStore((state) => state.setMobileKanbanMenuOpen);
  const isSearchOpen = useAppStore((state) => state.mobileKanban.isSearchOpen);
  const setSearchOpen = useAppStore((state) => state.setMobileKanbanSearchOpen);
  const handleOpenQuickChat = useQuickChatLauncher(workspaceId);
  const handleOpenQuickTerminal = useQuickTerminalLauncher(workspaceId);

  const toggleSearch = () => {
    const next = !isSearchOpen;
    setSearchOpen(next);
    // Clear any active query when collapsing so results aren't filtered by a hidden search.
    if (!next) onSearchChange?.("");
  };

  return (
    <>
      <PageTopbar
        title={title}
        leading={<MobileBrandLink workspaceId={workspaceId} />}
        showStatusTrigger={false}
        // The brand link is this bar's home affordance; a home crumb on top
        // of it would say the same thing twice.
        homeAffordance="none"
        // The strip is this bar's flexible zone, so the bar's leftover width has
        // to reach it. Left with the lead zone it would sit in the empty middle
        // and the `flex-1` strip, whose base size is zero, would render at zero
        // width with the actions unreachable behind it.
        freeWidth="actions"
        // The action strip scrolls rather than pushing the title crumb off the
        // bar, so this cluster has to be the flexible one in the right zone.
        actionsClassName="min-w-0 flex-1 !shrink gap-2"
        actions={
          <MobileHeaderActions
            workspaceId={workspaceId}
            workspaceLabel={workspaceLabel}
            currentPage={currentPage}
            onSearchChange={onSearchChange}
            isSearchOpen={isSearchOpen}
            handleOpenQuickChat={handleOpenQuickChat}
            handleOpenQuickTerminal={handleOpenQuickTerminal}
            toggleSearch={toggleSearch}
            taskListingControls={taskListingControls}
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
      />
    </>
  );
}
