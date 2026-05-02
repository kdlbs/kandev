"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { ToggleGroup, ToggleGroupItem } from "@kandev/ui/toggle-group";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  IconPlus,
  IconSettings,
  IconList,
  IconLayoutKanban,
  IconMenu2,
  IconChartBar,
  IconTimeline,
  IconStethoscope,
  IconDots,
  IconSparkles,
  IconAlertTriangle,
} from "@tabler/icons-react";
import { ImproveKandevDialog } from "@/components/improve-kandev-dialog";
import { IntegrationsMenu } from "@/components/integrations/integrations-menu";
import { PageTopbar } from "@/components/page-topbar";
import { KanbanDisplayDropdown } from "../kanban-display-dropdown";
import { ReleaseNotesDialog } from "../release-notes/release-notes-dialog";
import { HealthIndicatorButton, HealthIssuesDialog } from "../system-health/health-indicator";
import { TaskSearchInput } from "./task-search-input";
import { QuickChatButton } from "@/components/task/quick-chat-button";
import { KanbanHeaderMobile } from "./kanban-header-mobile";
import { MobileMenuSheet } from "./mobile-menu-sheet";
import { linkToTask, linkToTasks } from "@/lib/links";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useAppStore } from "@/components/state-provider";
import { useKanbanDisplaySettings } from "@/hooks/use-kanban-display-settings";
import { useReleaseNotes } from "@/hooks/use-release-notes";
import { useSystemHealthIndicator } from "@/hooks/use-system-health-indicator";
import type { ComponentProps, RefObject } from "react";

type KanbanHeaderProps = {
  onCreateTask: () => void;
  workspaceId?: string;
  currentPage?: "kanban" | "tasks";
  searchQuery?: string;
  onSearchChange?: (query: string) => void;
  isSearchLoading?: boolean;
};

type ViewToggleItem = {
  value: string;
  icon: typeof IconLayoutKanban;
  label: string;
};

type HeaderUtilityMenuProps = {
  showReleaseNotesButton: boolean;
  onOpenReleaseNotes: () => void;
  showHealthIndicator: boolean;
  onOpenHealthDialog: () => void;
  showStatsLink?: boolean;
  buttonSize?: ComponentProps<typeof Button>["size"];
};

const VIEW_TOGGLE_ITEMS: ViewToggleItem[] = [
  { value: "kanban", icon: IconLayoutKanban, label: "Kanban" },
  { value: "pipeline", icon: IconTimeline, label: "Pipeline" },
  { value: "list", icon: IconList, label: "List" },
];

const WORKBENCH_TOPBAR_CLASSNAME = "h-10 px-3 py-1";
const DESKTOP_HEADER_NARROW_PX = 1100;

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

function BoardUtilitiesMenu({
  showReleaseNotesButton,
  onOpenReleaseNotes,
  showHealthIndicator,
  onOpenHealthDialog,
  showStatsLink = true,
  buttonSize = "icon",
}: HeaderUtilityMenuProps) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="outline"
          size={buttonSize}
          className="cursor-pointer"
          aria-label="Utilities"
        >
          <IconDots className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-52">
        <DropdownMenuLabel>Utilities</DropdownMenuLabel>
        {showReleaseNotesButton && (
          <DropdownMenuItem onClick={onOpenReleaseNotes} className="cursor-pointer">
            <IconSparkles className="h-4 w-4" />
            Release notes
          </DropdownMenuItem>
        )}
        <DropdownMenuItem onClick={onOpenHealthDialog} className="cursor-pointer">
          <IconAlertTriangle
            className={`h-4 w-4 ${showHealthIndicator ? "text-warning" : "text-muted-foreground"}`}
          />
          {showHealthIndicator ? "Health issues" : "System health"}
        </DropdownMenuItem>
        {showStatsLink && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem asChild className="cursor-pointer">
              <Link href="/stats">
                <IconChartBar className="h-4 w-4" />
                Stats
              </Link>
            </DropdownMenuItem>
          </>
        )}
        <DropdownMenuItem asChild className="cursor-pointer">
          <Link href="/settings">
            <IconSettings className="h-4 w-4" />
            Settings
          </Link>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function ImproveKandevTopbarButton({
  workspaceId,
  buttonSize = "icon-lg",
}: {
  workspaceId: string | undefined;
  buttonSize?: ComponentProps<typeof Button>["size"];
}) {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  return (
    <>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="outline"
            size={buttonSize}
            onClick={() => setOpen(true)}
            className="cursor-pointer"
            data-testid="improve-kandev-button"
          >
            <IconStethoscope className="h-4 w-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>Improve KanDev</TooltipContent>
      </Tooltip>
      <ImproveKandevDialog
        open={open}
        onOpenChange={setOpen}
        workspaceId={workspaceId ?? null}
        onSuccess={(task) => router.push(linkToTask(task.id))}
      />
    </>
  );
}

function HomeLeftActions({ workspaceId }: { workspaceId?: string }) {
  return (
    <>
      <Button asChild variant="outline" size="lg" className="cursor-pointer">
        <Link href="/stats" aria-label="Stats">
          <IconChartBar className="h-4 w-4" />
          Stats
        </Link>
      </Button>
      <ImproveKandevTopbarButton workspaceId={workspaceId} />
      <IntegrationsMenu />
    </>
  );
}

function WorkspaceLeftActions({ workspaceId }: { workspaceId?: string }) {
  return (
    <>
      <ImproveKandevTopbarButton workspaceId={workspaceId} />
      <IntegrationsMenu />
    </>
  );
}

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
  onCreateTask,
  workspaceId,
  title,
  workspaceLabel,
  searchQuery,
  onSearchChange,
  isSearchLoading,
  toggleValue,
  handleViewChange,
  setMenuOpen,
  showHealthIndicator,
  onOpenHealthDialog,
}: {
  onCreateTask: () => void;
  workspaceId?: string;
  title: string;
  workspaceLabel: string;
  searchQuery: string;
  onSearchChange?: (query: string) => void;
  isSearchLoading: boolean;
  toggleValue: string;
  handleViewChange: (value: string) => void;
  setMenuOpen: (open: boolean) => void;
  showHealthIndicator: boolean;
  onOpenHealthDialog: () => void;
}) {
  const isHome = title === "Home";

  return (
    <PageTopbar
      title={title}
      subtitle={workspaceLabel}
      className={WORKBENCH_TOPBAR_CLASSNAME}
      variant={isHome ? "root" : "breadcrumb"}
      leftActions={
        isHome ? (
          <HomeLeftActions workspaceId={workspaceId} />
        ) : (
          <WorkspaceLeftActions workspaceId={workspaceId} />
        )
      }
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
          <Button
            onClick={onCreateTask}
            size="lg"
            className="cursor-pointer"
            data-testid="create-task-button"
          >
            <IconPlus className="h-4 w-4" />
            <span className="hidden sm:inline ml-1">Add task</span>
          </Button>
          <QuickChatButton workspaceId={workspaceId} size="lg" />
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
  onCreateTask,
  workspaceId,
  title,
  workspaceLabel,
  searchQuery,
  onSearchChange,
  isSearchLoading,
  toggleValue,
  handleViewChange,
  showReleaseNotesButton,
  onOpenReleaseNotes,
  showHealthIndicator,
  onOpenHealthDialog,
}: {
  onCreateTask: () => void;
  workspaceId?: string;
  title: string;
  workspaceLabel: string;
  searchQuery: string;
  onSearchChange?: (query: string) => void;
  isSearchLoading: boolean;
  toggleValue: string;
  handleViewChange: (value: string) => void;
  showReleaseNotesButton: boolean;
  onOpenReleaseNotes: () => void;
  showHealthIndicator: boolean;
  onOpenHealthDialog: () => void;
}) {
  const headerRef = useRef<HTMLElement>(null);
  const isNarrow = useIsHeaderNarrow(headerRef);
  const searchInput = onSearchChange ? (
    <TaskSearchInput
      value={searchQuery}
      onChange={onSearchChange}
      placeholder="Search tasks..."
      isLoading={isSearchLoading}
      className="w-72 xl:w-80 [&_input]:h-8"
    />
  ) : null;
  const isHome = title === "Home";
  const centerSearch =
    isHome && searchInput && !isNarrow ? (
      <div data-testid="kanban-header-search">{searchInput}</div>
    ) : null;
  const leftActions = isHome ? (
    <HomeLeftActions workspaceId={workspaceId} />
  ) : (
    <WorkspaceLeftActions workspaceId={workspaceId} />
  );

  return (
    <PageTopbar
      ref={headerRef}
      title={title}
      subtitle={workspaceLabel}
      center={centerSearch}
      className={WORKBENCH_TOPBAR_CLASSNAME}
      variant={isHome ? "root" : "breadcrumb"}
      leftActions={leftActions}
      actions={
        <>
          {!isHome && searchInput}
          <Button
            onClick={onCreateTask}
            size="lg"
            className="cursor-pointer"
            data-testid="create-task-button"
          >
            <IconPlus className="h-4 w-4" />
            Add task
          </Button>
          <QuickChatButton workspaceId={workspaceId} size="lg" />
          <TooltipProvider>
            <ViewToggleGroup toggleValue={toggleValue} onValueChange={handleViewChange} size="lg" />
          </TooltipProvider>
          <KanbanDisplayDropdown triggerSize="icon-lg" />
          <HealthIndicatorButton
            hasIssues={showHealthIndicator}
            onClick={onOpenHealthDialog}
            size="icon-lg"
          />
          <BoardUtilitiesMenu
            showReleaseNotesButton={showReleaseNotesButton}
            onOpenReleaseNotes={onOpenReleaseNotes}
            showHealthIndicator={showHealthIndicator}
            onOpenHealthDialog={onOpenHealthDialog}
            showStatsLink={!isHome}
            buttonSize="icon-lg"
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
  onCreateTask,
  workspaceId,
  currentPage = "kanban",
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

  const indicatorProps = {
    showReleaseNotesButton: releaseNotes.showTopbarButton,
    onOpenReleaseNotes: releaseNotes.openDialog,
    showHealthIndicator: healthIndicator.hasIssues,
    onOpenHealthDialog: healthIndicator.openDialog,
  };
  const sharedSearch = { searchQuery, onSearchChange, isSearchLoading };
  const sharedActions = { onCreateTask, workspaceId };

  const renderHeader = () => {
    if (isMobile) {
      return (
        <KanbanHeaderMobile
          workspaceId={workspaceId}
          currentPage={currentPage}
          title={title}
          workspaceLabel={workspaceLabel}
          {...sharedSearch}
          {...indicatorProps}
        />
      );
    }
    if (isTablet) {
      return (
        <>
          <TabletHeader
            {...sharedActions}
            title={title}
            workspaceLabel={workspaceLabel}
            {...sharedSearch}
            toggleValue={toggleValue}
            handleViewChange={handleViewChange}
            setMenuOpen={setMenuOpen}
            {...indicatorProps}
          />
          <MobileMenuSheet
            open={isMenuOpen}
            onOpenChange={setMenuOpen}
            workspaceId={workspaceId}
            currentPage={currentPage}
            {...sharedSearch}
            {...indicatorProps}
          />
        </>
      );
    }
    return (
      <DesktopHeader
        {...sharedActions}
        title={title}
        workspaceLabel={workspaceLabel}
        {...sharedSearch}
        toggleValue={toggleValue}
        handleViewChange={handleViewChange}
        {...indicatorProps}
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
