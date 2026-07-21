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
  IconTimeline,
} from "@tabler/icons-react";
import { PageTopbar } from "@/components/page-topbar";
import { KanbanDisplayDropdown } from "../kanban-display-dropdown";
import { ReleaseNotesDialog } from "../release-notes/release-notes-dialog";
import { HealthIndicatorButton, HealthIssuesDialog } from "../system-health/health-indicator";
import { TaskSearchInput } from "./task-search-input";
import { KanbanHeaderMobile } from "./kanban-header-mobile";
import { MainTopBarPluginActions } from "./main-top-bar-plugin-actions";
import { MobileMenuSheet } from "./mobile-menu-sheet";
import { linkToTasks } from "@/lib/links";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useAppStore } from "@/components/state-provider";
import { useKanbanDisplaySettings } from "@/hooks/use-kanban-display-settings";
import { useReleaseNotes } from "@/hooks/use-release-notes";
import { useSystemHealthIndicator } from "@/hooks/use-system-health-indicator";
import { useQuickChatLauncher } from "@/hooks/use-quick-chat-launcher";
import type { ComponentProps, RefObject } from "react";

type KanbanHeaderProps = {
  workspaceId?: string;
  currentPage?: "kanban" | "tasks";
  hideTitle?: boolean;
  searchQuery?: string;
  onSearchChange?: (query: string) => void;
  isSearchLoading?: boolean;
};

type ViewToggleItem = {
  value: string;
  icon: typeof IconLayoutKanban;
  label: string;
};

const VIEW_TOGGLE_ITEMS: ViewToggleItem[] = [
  { value: "kanban", icon: IconLayoutKanban, label: "Kanban" },
  { value: "pipeline", icon: IconTimeline, label: "Pipeline" },
  { value: "list", icon: IconList, label: "List" },
];

const WORKBENCH_TOPBAR_CLASSNAME = "h-12 border-b-0 px-3 py-2";
const DESKTOP_HEADER_NARROW_PX = 800;

function getWorkspaceLabel(
  workspaces: Array<{ id: string; name: string }>,
  activeWorkspaceId: string | null,
): string {
  if (!activeWorkspaceId) return "All workspaces";
  return workspaces.find((workspace) => workspace.id === activeWorkspaceId)?.name ?? "Workspace";
}

function getHeaderTitle(currentPage: string): string {
  return currentPage === "tasks" ? "Tasks" : "Home";
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
  return (
    <ToggleGroup
      type="single"
      value={toggleValue}
      onValueChange={onValueChange}
      variant="outline"
      size={size}
      className={className}
    >
      {VIEW_TOGGLE_ITEMS.map(({ value, icon: Icon, label }) => (
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
            <TooltipContent>{label}</TooltipContent>
          </Tooltip>
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}

function getToggleValue(currentPage: string, kanbanViewMode: string | null): string {
  if (currentPage === "tasks") return "list";
  if (kanbanViewMode === "graph2") return "pipeline";
  return "kanban";
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
  hideTitle,
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
  hideTitle?: boolean;
}) {
  const isHome = title === "Home";
  const handleOpenQuickChat = useQuickChatLauncher(workspaceId);

  return (
    <PageTopbar
      title={hideTitle ? "" : title}
      subtitle={hideTitle ? undefined : workspaceLabel}
      backLabel={hideTitle || isHome ? "" : "Kandev"}
      className={WORKBENCH_TOPBAR_CLASSNAME}
      variant={hideTitle || isHome ? "root" : "breadcrumb"}
      actionsClassName="gap-2"
      actions={
        <>
          {onSearchChange && (
            <TaskSearchInput
              value={searchQuery}
              onChange={onSearchChange}
              placeholder="Search..."
              isLoading={isSearchLoading}
              className="hidden md:flex w-48 lg:w-56 [&_input]:h-8"
            />
          )}
          <MainTopBarPluginActions
            workspaceId={workspaceId}
            workspaceLabel={workspaceLabel}
            currentPage={currentPage}
          />
          {workspaceId && (
            <Button
              variant="outline"
              size="icon-lg"
              onClick={handleOpenQuickChat}
              className="cursor-pointer"
              aria-label="Quick Chat"
              data-testid="tablet-quick-chat-button"
            >
              <IconMessageCircle className="h-4 w-4" />
            </Button>
          )}
          <TooltipProvider>
            <ViewToggleGroup toggleValue={toggleValue} onValueChange={handleViewChange} size="lg" />
          </TooltipProvider>
          <KanbanDisplayDropdown triggerSize="icon-lg" />
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
            <span className="sr-only">Open menu</span>
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
  hideTitle,
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
  hideTitle?: boolean;
}) {
  const headerRef = useRef<HTMLElement>(null);
  const isNarrow = useIsHeaderNarrow(headerRef);
  const searchInput = onSearchChange ? (
    <TaskSearchInput
      value={searchQuery}
      onChange={onSearchChange}
      placeholder="Search tasks..."
      isLoading={isSearchLoading}
      className={`${isNarrow ? "w-44" : "w-72 xl:w-80"} [&_input]:h-8`}
    />
  ) : null;
  const isHome = title === "Home";
  const centerSearch =
    searchInput && !isNarrow ? <div data-testid="kanban-header-search">{searchInput}</div> : null;
  const actionsSearch = isNarrow ? searchInput : null;

  return (
    <PageTopbar
      ref={headerRef}
      title={hideTitle ? "" : title}
      subtitle={hideTitle ? undefined : workspaceLabel}
      backLabel={hideTitle || isHome ? "" : "Kandev"}
      center={centerSearch}
      className={WORKBENCH_TOPBAR_CLASSNAME}
      variant={hideTitle || isHome ? "root" : "breadcrumb"}
      actions={
        <>
          {actionsSearch}
          <MainTopBarPluginActions
            workspaceId={workspaceId}
            workspaceLabel={workspaceLabel}
            currentPage={currentPage}
          />
          <TooltipProvider>
            <ViewToggleGroup toggleValue={toggleValue} onValueChange={handleViewChange} size="lg" />
          </TooltipProvider>
          <KanbanDisplayDropdown triggerSize="icon-lg" />
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

function useHeaderViewChange(
  currentPage: string,
  workspaceId: string | undefined,
  onViewModeChange: (mode: string) => void,
) {
  const router = useRouter();
  return (value: string) => {
    if (value === "list") {
      if (currentPage !== "tasks") router.push(linkToTasks(workspaceId));
    } else if (value === "kanban") {
      if (currentPage !== "kanban") router.push("/");
      onViewModeChange("");
    } else if (value === "pipeline") {
      if (currentPage !== "kanban") router.push("/");
      onViewModeChange("graph2");
    }
  };
}

export function KanbanHeader({
  workspaceId,
  currentPage = "kanban",
  hideTitle = false,
  searchQuery = "",
  onSearchChange,
  isSearchLoading = false,
}: KanbanHeaderProps) {
  const { isMobile, isTablet } = useResponsiveBreakpoint();
  const isMenuOpen = useAppStore((state) => state.mobileKanban.isMenuOpen);
  const setMenuOpen = useAppStore((state) => state.setMobileKanbanMenuOpen);
  const { kanbanViewMode, onViewModeChange, workspaces, activeWorkspaceId } =
    useKanbanDisplaySettings();
  const releaseNotes = useReleaseNotes();
  const healthIndicator = useSystemHealthIndicator();
  const toggleValue = getToggleValue(currentPage, kanbanViewMode);
  const handleViewChange = useHeaderViewChange(currentPage, workspaceId, onViewModeChange);
  const title = getHeaderTitle(currentPage);
  const workspaceLabel = getWorkspaceLabel(workspaces, activeWorkspaceId);

  const healthProps = {
    showHealthIndicator: healthIndicator.hasIssues,
    onOpenHealthDialog: healthIndicator.openDialog,
  };
  const sharedSearch = { searchQuery, onSearchChange, isSearchLoading };

  const renderHeader = () => {
    if (isMobile) {
      return (
        <KanbanHeaderMobile
          workspaceId={workspaceId}
          currentPage={currentPage}
          title={title}
          workspaceLabel={workspaceLabel}
          hideTitle={hideTitle}
          {...sharedSearch}
          {...healthProps}
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
            hideTitle={hideTitle}
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
            {...healthProps}
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
        hideTitle={hideTitle}
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
