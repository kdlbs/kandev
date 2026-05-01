"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@kandev/ui/button";
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
  IconBrandGithub,
  IconTicket,
  IconHexagon,
  IconStethoscope,
} from "@tabler/icons-react";
import { useJiraAvailable } from "@/components/jira/my-jira/use-jira-availability";
import { useLinearAvailable } from "@/components/linear/use-linear-availability";
import { ImproveKandevDialog } from "@/components/improve-kandev-dialog";
import { linkToTask } from "@/lib/links";
import { KanbanDisplayDropdown } from "../kanban-display-dropdown";
import { ReleaseNotesButton } from "../release-notes/release-notes-button";
import { ReleaseNotesDialog } from "../release-notes/release-notes-dialog";
import { HealthIndicatorButton, HealthIssuesDialog } from "../system-health/health-indicator";
import { TaskSearchInput } from "./task-search-input";
import { QuickChatButton } from "@/components/task/quick-chat-button";
import { KanbanHeaderMobile } from "./kanban-header-mobile";
import { MobileMenuSheet } from "./mobile-menu-sheet";
import { linkToTasks } from "@/lib/links";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useAppStore } from "@/components/state-provider";
import { useKanbanDisplaySettings } from "@/hooks/use-kanban-display-settings";
import { useGitHubStatus } from "@/hooks/domains/github/use-github-status";
import { useReleaseNotes } from "@/hooks/use-release-notes";
import { useSystemHealthIndicator } from "@/hooks/use-system-health-indicator";

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

const VIEW_TOGGLE_ITEMS: ViewToggleItem[] = [
  { value: "kanban", icon: IconLayoutKanban, label: "Kanban" },
  { value: "pipeline", icon: IconTimeline, label: "Pipeline" },
  { value: "list", icon: IconList, label: "List" },
];

function GitHubTopbarButton() {
  const { status } = useGitHubStatus();
  if (!status?.authenticated) return null;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button variant="outline" size="icon" asChild className="cursor-pointer">
          <Link href="/github">
            <IconBrandGithub className="h-4 w-4" />
          </Link>
        </Button>
      </TooltipTrigger>
      <TooltipContent>GitHub</TooltipContent>
    </Tooltip>
  );
}

function JiraTopbarButton({ workspaceId }: { workspaceId: string | undefined }) {
  const available = useJiraAvailable(workspaceId);
  if (!available) return null;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button variant="outline" size="icon" asChild className="cursor-pointer">
          <Link href="/jira">
            <IconTicket className="h-4 w-4" />
          </Link>
        </Button>
      </TooltipTrigger>
      <TooltipContent>Jira</TooltipContent>
    </Tooltip>
  );
}

function LinearTopbarButton({ workspaceId }: { workspaceId: string | undefined }) {
  const available = useLinearAvailable(workspaceId);
  if (!available) return null;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button variant="outline" size="icon" asChild className="cursor-pointer">
          <Link href="/linear">
            <IconHexagon className="h-4 w-4" />
          </Link>
        </Button>
      </TooltipTrigger>
      <TooltipContent>Linear</TooltipContent>
    </Tooltip>
  );
}

function ImproveKandevTopbarButton({ workspaceId }: { workspaceId: string | undefined }) {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  return (
    <>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="outline"
            size="icon"
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

function ViewToggleGroup({
  toggleValue,
  onValueChange,
  className,
  itemClassName,
}: {
  toggleValue: string;
  onValueChange: (value: string) => void;
  className?: string;
  itemClassName?: string;
}) {
  return (
    <ToggleGroup
      type="single"
      value={toggleValue}
      onValueChange={onValueChange}
      variant="outline"
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

function TabletHeader({
  onCreateTask,
  workspaceId,
  searchQuery,
  onSearchChange,
  isSearchLoading,
  toggleValue,
  handleViewChange,
  setMenuOpen,
  showReleaseNotesButton,
  onOpenReleaseNotes,
  showHealthIndicator,
  onOpenHealthDialog,
}: {
  onCreateTask: () => void;
  workspaceId?: string;
  searchQuery: string;
  onSearchChange?: (query: string) => void;
  isSearchLoading: boolean;
  toggleValue: string;
  handleViewChange: (value: string) => void;
  setMenuOpen: (open: boolean) => void;
  showReleaseNotesButton: boolean;
  onOpenReleaseNotes: () => void;
  showHealthIndicator: boolean;
  onOpenHealthDialog: () => void;
}) {
  return (
    <header className="flex items-center justify-between p-4 pb-3 gap-3">
      <div className="flex items-center gap-3 flex-shrink-0">
        <Link href="/" className="text-xl font-bold hover:opacity-80">
          KanDev
        </Link>
        <TooltipProvider>
          <GitHubTopbarButton />
          <JiraTopbarButton workspaceId={workspaceId} />
          <LinearTopbarButton workspaceId={workspaceId} />
          <ImproveKandevTopbarButton workspaceId={workspaceId} />
        </TooltipProvider>
      </div>
      {onSearchChange && (
        <TaskSearchInput
          value={searchQuery}
          onChange={onSearchChange}
          placeholder="Search..."
          isLoading={isSearchLoading}
          className="flex-1 max-w-[200px]"
        />
      )}
      <div className="flex items-center gap-2">
        <Button
          onClick={onCreateTask}
          size="lg"
          className="cursor-pointer"
          data-testid="create-task-button"
        >
          <IconPlus className="h-4 w-4" />
          <span className="hidden sm:inline ml-1">Add task</span>
        </Button>
        <QuickChatButton workspaceId={workspaceId} />
        <TooltipProvider>
          <ViewToggleGroup
            toggleValue={toggleValue}
            onValueChange={handleViewChange}
            className="h-8"
            itemClassName="h-8 w-8"
          />
          {showReleaseNotesButton && <ReleaseNotesButton hasUnseen onClick={onOpenReleaseNotes} />}
          <HealthIndicatorButton hasIssues={showHealthIndicator} onClick={onOpenHealthDialog} />
        </TooltipProvider>
        <Button
          variant="outline"
          size="icon-lg"
          onClick={() => setMenuOpen(true)}
          className="cursor-pointer"
        >
          <IconMenu2 className="h-4 w-4" />
          <span className="sr-only">Open menu</span>
        </Button>
      </div>
    </header>
  );
}

// Width below which the centered search input no longer fits between the
// left ("KanDev" + GitHub/Jira/Stats) and right (Add task + chat + view
// toggle + indicators + display + settings) action groups.
const DESKTOP_HEADER_NARROW_PX = 1100;

function useIsHeaderNarrow(ref: React.RefObject<HTMLElement | null>): boolean {
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

function DesktopHeader({
  onCreateTask,
  workspaceId,
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
  // The search input is absolutely centered, so the left/right action groups
  // can grow into it and overlap. Hide the search when the header gets too
  // narrow to fit all three regions side-by-side (e.g. when the kanban preview
  // panel is open and squeezes the board area).
  const headerRef = useRef<HTMLElement>(null);
  const isNarrow = useIsHeaderNarrow(headerRef);
  const showSearch = !!onSearchChange && !isNarrow;
  return (
    <header ref={headerRef} className="relative flex items-center justify-between p-4 pb-3">
      <div className="flex items-center gap-5">
        <Link href="/" className="text-2xl font-bold hover:opacity-80">
          KanDev
        </Link>
        <div className="flex items-center gap-3">
          <TooltipProvider>
            <GitHubTopbarButton />
            <JiraTopbarButton workspaceId={workspaceId} />
            <LinearTopbarButton workspaceId={workspaceId} />
            <Tooltip>
              <TooltipTrigger asChild>
                <Button variant="outline" size="icon" asChild className="cursor-pointer">
                  <Link href="/stats">
                    <IconChartBar className="h-4 w-4" />
                  </Link>
                </Button>
              </TooltipTrigger>
              <TooltipContent>Stats</TooltipContent>
            </Tooltip>
            <ImproveKandevTopbarButton workspaceId={workspaceId} />
          </TooltipProvider>
        </div>
      </div>
      {showSearch && (
        <div className="absolute left-1/2 -translate-x-1/2" data-testid="kanban-header-search">
          <TaskSearchInput
            value={searchQuery}
            onChange={onSearchChange}
            placeholder="Search tasks..."
            isLoading={isSearchLoading}
            className="w-64"
          />
        </div>
      )}
      <div className="flex items-center gap-3">
        <Button onClick={onCreateTask} className="cursor-pointer" data-testid="create-task-button">
          <IconPlus className="h-4 w-4" />
          Add task
        </Button>
        <QuickChatButton workspaceId={workspaceId} />
        <TooltipProvider>
          <ViewToggleGroup toggleValue={toggleValue} onValueChange={handleViewChange} />
        </TooltipProvider>
        {showReleaseNotesButton && <ReleaseNotesButton hasUnseen onClick={onOpenReleaseNotes} />}
        <HealthIndicatorButton hasIssues={showHealthIndicator} onClick={onOpenHealthDialog} />
        <KanbanDisplayDropdown />
        <Link href="/settings" className="cursor-pointer">
          <Button variant="outline" className="cursor-pointer gap-2">
            <IconSettings className="h-4 w-4" />
            <span className="hidden 2xl:inline">Settings</span>
          </Button>
        </Link>
      </div>
    </header>
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
  const { kanbanViewMode, onViewModeChange } = useKanbanDisplaySettings();
  const releaseNotes = useReleaseNotes();
  const healthIndicator = useSystemHealthIndicator();
  const toggleValue = getToggleValue(currentPage, kanbanViewMode);
  const handleViewChange = useHeaderViewChange(currentPage, workspaceId, onViewModeChange);

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
          />
        </>
      );
    }
    return (
      <DesktopHeader
        {...sharedActions}
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
