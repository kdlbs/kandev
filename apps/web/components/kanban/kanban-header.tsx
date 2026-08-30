"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "@/lib/routing/client-router";
import { Button } from "@kandev/ui/button";
import { ToggleGroup, ToggleGroupItem } from "@kandev/ui/toggle-group";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  IconList,
  IconLayoutKanban,
  IconMenu2,
  IconMessageCircle,
  IconTerminal2,
  IconTimeline,
} from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { PageTopbar } from "@/components/page-topbar";
import { KanbanDisplayDropdown } from "../kanban-display-dropdown";
import { usePluginTaskFilters } from "@/hooks/use-plugin-task-filters";
import { ReleaseNotesDialog } from "../release-notes/release-notes-dialog";
import { HealthIndicatorButton, HealthIssuesDialog } from "../system-health/health-indicator";
import { TaskSearchInput } from "./task-search-input";
import { KanbanHeaderMobile } from "./kanban-header-mobile";
import { MainTopBarPluginActions } from "./main-top-bar-plugin-actions";
import { MobileMenuSheet } from "./mobile-menu-sheet";
import type { TasksListDisplayOptions } from "./mobile-menu-task-list-options";
import { linkToTaskOverview, linkToTasks } from "@/lib/links";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useAppStore } from "@/components/state-provider";
import { QuickChatActivityIndicator } from "@/components/quick-chat/quick-chat-activity-indicator";
import { useQuickChatActivity } from "@/components/quick-chat/use-quick-chat-activity";
import { useKanbanDisplaySettings } from "@/hooks/use-kanban-display-settings";
import { useReleaseNotes } from "@/hooks/use-release-notes";
import { useSystemHealthIndicator } from "@/hooks/use-system-health-indicator";
import { useQuickChatLauncher } from "@/hooks/use-quick-chat-launcher";
import { useQuickTerminalLauncher } from "@/hooks/use-quick-terminal-launcher";
import { TopbarMetrics } from "@/components/system-metrics/topbar-metrics";
import type { ComponentProps, RefObject } from "react";

type KanbanHeaderProps = {
  workspaceId?: string;
  currentPage?: "kanban" | "tasks";
  searchQuery?: string;
  onSearchChange?: (query: string) => void;
  isSearchLoading?: boolean;
  tasksListOptions?: TasksListDisplayOptions;
};

type ViewToggleItem = {
  value: string;
  icon: typeof IconLayoutKanban;
  /** Resolved at render — a `t()` here would freeze at the boot locale. */
  labelKey: string;
};

const VIEW_TOGGLE_ITEMS: ViewToggleItem[] = [
  { value: "kanban", icon: IconLayoutKanban, labelKey: "kanban:kanban" },
  { value: "pipeline", icon: IconTimeline, labelKey: "kanban:pipeline" },
  { value: "list", icon: IconList, labelKey: "kanban:list" },
];

const DESKTOP_HEADER_NARROW_PX = 800;

function getWorkspaceLabel(
  workspaces: Array<{ id: string; name: string }>,
  activeWorkspaceId: string | null,
  t: TFunction,
): string {
  if (!activeWorkspaceId) return t("kanban:allWorkspaces");
  return (
    workspaces.find((workspace) => workspace.id === activeWorkspaceId)?.name ??
    t("common:workspace")
  );
}

/**
 * The header title is display copy, so it must not double as the "is this the
 * home page?" flag — `title === "Home"` was true only in English and silently
 * picked the wrong branch in every other locale. `currentPage` is the
 * discriminant; the title is derived from it.
 */
function isHomePage(currentPage: string): boolean {
  return currentPage !== "tasks";
}

function getHeaderTitle(currentPage: string, t: TFunction): string {
  return isHomePage(currentPage) ? t("sidebar:home") : t("sidebar:tasks");
}

function toHeaderHealthProps(health: ReturnType<typeof useSystemHealthIndicator>) {
  return {
    showHealthIndicator: health.hasIssues,
    onOpenHealthDialog: health.openDialog,
  };
}

// Integrations / Stats / Office / Improve Kandev / Settings / Release notes
// have all moved to the unified AppSidebar (Fix 6). The kanban top bar now
// focuses on task-creation, view-toggle, kanban display, and search.

function ViewToggleGroup({
  toggleValue,
  onValueChange,
  size,
  className,
  itemClassName,
}: {
  toggleValue: string;
  onValueChange: (value: string) => void;
  size?: ComponentProps<typeof ToggleGroup>["size"];
  className?: string;
  itemClassName?: string;
}) {
  const { t } = useTranslation();
  return (
    <ToggleGroup
      type="single"
      value={toggleValue}
      onValueChange={onValueChange}
      variant="outline"
      size={size}
      className={className}
    >
      {VIEW_TOGGLE_ITEMS.map(({ value, icon: Icon, labelKey }) => (
        <ToggleGroupItem
          key={value}
          value={value}
          data-testid={`view-toggle-${value}`}
          className={`cursor-pointer data-[state=on]:bg-muted data-[state=on]:text-foreground ${itemClassName ?? ""}`}
        >
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="flex items-center justify-center">
                <Icon className="h-4 w-4" />
              </span>
            </TooltipTrigger>
            <TooltipContent>{t(labelKey)}</TooltipContent>
          </Tooltip>
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}

function useIsHeaderNarrow(ref: RefObject<HTMLElement | null>): boolean {
  const [isNarrow, setIsNarrow] = useState(false);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const update = () => setIsNarrow(el.clientWidth < DESKTOP_HEADER_NARROW_PX);
    update();
    const observer = new ResizeObserver(update);
    observer.observe(el);
    return () => observer.disconnect();
  }, [ref]);

  return isNarrow;
}

function TabletQuickActions({ workspaceId }: { workspaceId?: string }) {
  const { t } = useTranslation();
  const handleOpenQuickChat = useQuickChatLauncher(workspaceId);
  const { activity: quickChatActivity, label: quickChatLabel } = useQuickChatActivity(workspaceId);
  const handleOpenQuickTerminal = useQuickTerminalLauncher(workspaceId);
  if (!workspaceId) return null;

  return (
    <>
      <Button
        variant="outline"
        size="icon-lg"
        onClick={handleOpenQuickTerminal}
        className="!size-11 cursor-pointer"
        aria-label={t("sidebar:quickTerminal")}
        data-testid="tablet-quick-terminal-button"
      >
        <IconTerminal2 className="h-4 w-4" />
      </Button>
      <Button
        variant="outline"
        size="icon-lg"
        onClick={handleOpenQuickChat}
        className="!size-11 cursor-pointer"
        aria-label={quickChatLabel}
        data-testid="tablet-quick-chat-button"
      >
        <span className="relative flex">
          <IconMessageCircle className="h-4 w-4" />
          <QuickChatActivityIndicator activity={quickChatActivity} />
        </span>
      </Button>
    </>
  );
}

function TabletHeader({
  title,
  workspaceLabel,
  workspaceId,
  currentPage,
  searchQuery,
  onSearchChange,
  isSearchLoading,
  toggleValue,
  handleViewChange,
  setMenuOpen,
  showHealthIndicator,
  onOpenHealthDialog,
}: {
  title: string;
  workspaceLabel: string;
  workspaceId?: string;
  currentPage: "kanban" | "tasks";
  searchQuery: string;
  onSearchChange?: (query: string) => void;
  isSearchLoading: boolean;
  toggleValue: string;
  handleViewChange: (value: string) => void;
  setMenuOpen: (open: boolean) => void;
  showHealthIndicator: boolean;
  onOpenHealthDialog: () => void;
}) {
  const { t } = useTranslation();
  const pluginTaskFilters = usePluginTaskFilters();

  return (
    <PageTopbar
      title={title}
      // 44px floor: this bar keeps its icon-lg touch controls above the
      // token's 40px. The md: variant is required — tablets render at md+
      // where the token's md:min-h-10 would beat an unprefixed floor.
      className="md:min-h-11"
      actionsClassName="gap-2"
      actions={
        <>
          {onSearchChange && (
            <TaskSearchInput
              value={searchQuery}
              onChange={onSearchChange}
              placeholder={t("kanban:searchPlaceholder")}
              isLoading={isSearchLoading}
              className="hidden md:flex w-48 lg:w-56 [&_input]:h-8"
            />
          )}
          <MainTopBarPluginActions
            workspaceId={workspaceId}
            workspaceLabel={workspaceLabel}
            currentPage={currentPage}
          />
          <TopbarMetrics size="lg" />
          <TabletQuickActions workspaceId={workspaceId} />
          <TooltipProvider>
            <ViewToggleGroup toggleValue={toggleValue} onValueChange={handleViewChange} size="lg" />
          </TooltipProvider>
          <KanbanDisplayDropdown
            triggerSize="icon-lg"
            currentPage={currentPage}
            pluginFilters={pluginTaskFilters.filters}
            pluginFilterSelections={pluginTaskFilters.selections}
            onPluginFilterChange={pluginTaskFilters.setFilterSelection}
          />
          <HealthIndicatorButton
            hasIssues={showHealthIndicator}
            onClick={onOpenHealthDialog}
            size="icon-lg"
          />
          <Button
            variant="outline"
            size="icon-lg"
            onClick={() => setMenuOpen(true)}
            className="cursor-pointer"
          >
            <IconMenu2 className="h-4 w-4" />
            <span className="sr-only">{t("kanban:openMenu")}</span>
          </Button>
        </>
      }
    />
  );
}

function DesktopHeader({
  title,
  workspaceLabel,
  workspaceId,
  currentPage,
  searchQuery,
  onSearchChange,
  isSearchLoading,
  toggleValue,
  handleViewChange,
  showHealthIndicator,
  onOpenHealthDialog,
}: {
  title: string;
  workspaceLabel: string;
  workspaceId?: string;
  currentPage: "kanban" | "tasks";
  searchQuery: string;
  onSearchChange?: (query: string) => void;
  isSearchLoading: boolean;
  toggleValue: string;
  handleViewChange: (value: string) => void;
  showHealthIndicator: boolean;
  onOpenHealthDialog: () => void;
}) {
  const { t } = useTranslation();
  const headerRef = useRef<HTMLElement>(null);
  const isNarrow = useIsHeaderNarrow(headerRef);
  const pluginTaskFilters = usePluginTaskFilters();
  const searchInput = onSearchChange ? (
    <TaskSearchInput
      value={searchQuery}
      onChange={onSearchChange}
      placeholder={t("kanban:searchTasksPlaceholder")}
      isLoading={isSearchLoading}
      className={`${isNarrow ? "w-44" : "w-72 xl:w-80"} [&_input]:h-8`}
    />
  ) : null;
  const centerSearch =
    searchInput && !isNarrow ? <div data-testid="kanban-header-search">{searchInput}</div> : null;
  const actionsSearch = isNarrow ? searchInput : null;

  return (
    <PageTopbar
      ref={headerRef}
      title={title}
      center={centerSearch}
      actions={
        <>
          {actionsSearch}
          <MainTopBarPluginActions
            workspaceId={workspaceId}
            workspaceLabel={workspaceLabel}
            currentPage={currentPage}
          />
          <TopbarMetrics size="lg" />
          <TooltipProvider>
            <ViewToggleGroup toggleValue={toggleValue} onValueChange={handleViewChange} size="lg" />
          </TooltipProvider>
          <KanbanDisplayDropdown
            triggerSize="icon-lg"
            currentPage={currentPage}
            pluginFilters={pluginTaskFilters.filters}
            pluginFilterSelections={pluginTaskFilters.selections}
            onPluginFilterChange={pluginTaskFilters.setFilterSelection}
          />
          <HealthIndicatorButton
            hasIssues={showHealthIndicator}
            onClick={onOpenHealthDialog}
            size="icon-lg"
          />
        </>
      }
    />
  );
}

function useHeaderView(
  currentPage: string,
  workspaceId: string | undefined,
  workflowId: string | null,
  onViewModeChange: (mode: string) => void,
) {
  const router = useRouter();
  return (value: string) => {
    if (value === "list") {
      onViewModeChange("list");
      if (currentPage !== "tasks") router.push(linkToTasks(workspaceId));
    } else if (value === "kanban") {
      onViewModeChange("kanban");
      if (currentPage !== "kanban")
        router.push(linkToTaskOverview({ workspaceId, workflowId: workflowId ?? undefined }));
    } else if (value === "pipeline") {
      onViewModeChange("pipeline");
      if (currentPage !== "kanban")
        router.push(linkToTaskOverview({ workspaceId, workflowId: workflowId ?? undefined }));
    }
  };
}

export function KanbanHeader({
  workspaceId,
  currentPage = "kanban",
  searchQuery = "",
  onSearchChange,
  isSearchLoading = false,
  tasksListOptions,
}: KanbanHeaderProps) {
  const { t } = useTranslation();
  const { isMobile, isTablet } = useResponsiveBreakpoint();
  const isMenuOpen = useAppStore((state) => state.mobileKanban.isMenuOpen);
  const setMenuOpen = useAppStore((state) => state.setMobileKanbanMenuOpen);
  const workflowId = useAppStore((state) => state.workflows.activeId);
  const display = useKanbanDisplaySettings();
  const { effectiveTaskListingView, onViewModeChange, workspaces, activeWorkspaceId } = display;
  const releaseNotes = useReleaseNotes();
  const healthIndicator = useSystemHealthIndicator();
  const toggleValue = currentPage === "tasks" ? "list" : effectiveTaskListingView;
  const handleViewChange = useHeaderView(currentPage, workspaceId, workflowId, onViewModeChange);
  const title = getHeaderTitle(currentPage, t);
  const workspaceLabel = getWorkspaceLabel(workspaces, activeWorkspaceId, t);

  const healthProps = toHeaderHealthProps(healthIndicator);
  const sharedSearch = { searchQuery, onSearchChange, isSearchLoading };

  const renderHeader = () => {
    if (isMobile) {
      return (
        <KanbanHeaderMobile
          workspaceId={workspaceId}
          currentPage={currentPage}
          title={title}
          workspaceLabel={workspaceLabel}
          {...sharedSearch}
          tasksListOptions={tasksListOptions}
        />
      );
    }
    if (isTablet) {
      return (
        <>
          <TabletHeader
            title={title}
            workspaceLabel={workspaceLabel}
            workspaceId={workspaceId}
            currentPage={currentPage}
            {...sharedSearch}
            toggleValue={toggleValue}
            handleViewChange={handleViewChange}
            setMenuOpen={setMenuOpen}
            {...healthProps}
          />
          <MobileMenuSheet
            open={isMenuOpen}
            onOpenChange={setMenuOpen}
            workspaceId={workspaceId}
            currentPage={currentPage}
            {...sharedSearch}
            tasksListOptions={tasksListOptions}
          />
        </>
      );
    }
    return (
      <DesktopHeader
        title={title}
        workspaceLabel={workspaceLabel}
        workspaceId={workspaceId}
        currentPage={currentPage}
        {...sharedSearch}
        toggleValue={toggleValue}
        handleViewChange={handleViewChange}
        {...healthProps}
      />
    );
  };

  return (
    <>
      {renderHeader()}
      {releaseNotes.hasNotes && (
        <ReleaseNotesDialog
          open={releaseNotes.dialogOpen}
          onOpenChange={releaseNotes.closeDialog}
          entries={releaseNotes.unseenEntries}
          latestVersion={releaseNotes.latestVersion}
        />
      )}
      <HealthIssuesDialog
        open={healthIndicator.dialogOpen}
        onOpenChange={healthIndicator.closeDialog}
        issues={healthIndicator.issues}
      />
    </>
  );
}
