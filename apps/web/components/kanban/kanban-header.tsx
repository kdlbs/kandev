"use client";

import Link from "next/link";
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
} from "@tabler/icons-react";
import { KanbanDisplayDropdown } from "../kanban-display-dropdown";
import { RefreshReviewsButton } from "../github/refresh-reviews-button";
import { ReleaseNotesButton } from "../release-notes/release-notes-button";
import { ReleaseNotesDialog } from "../release-notes/release-notes-dialog";
import { TaskSearchInput } from "./task-search-input";
import { KanbanHeaderMobile } from "./kanban-header-mobile";
import { MobileMenuSheet } from "./mobile-menu-sheet";
import { linkToTasks } from "@/lib/links";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useAppStore } from "@/components/state-provider";
import { useKanbanDisplaySettings } from "@/hooks/use-kanban-display-settings";
import { useReleaseNotes } from "@/hooks/use-release-notes";

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
  searchQuery,
  onSearchChange,
  isSearchLoading,
  toggleValue,
  handleViewChange,
  setMenuOpen,
  showReleaseNotesButton,
  onOpenReleaseNotes,
}: {
  onCreateTask: () => void;
  searchQuery: string;
  onSearchChange?: (query: string) => void;
  isSearchLoading: boolean;
  toggleValue: string;
  handleViewChange: (value: string) => void;
  setMenuOpen: (open: boolean) => void;
  showReleaseNotesButton: boolean;
  onOpenReleaseNotes: () => void;
}) {
  return (
    <header className="flex items-center justify-between p-4 pb-3 gap-3">
      <Link href="/" className="text-xl font-bold hover:opacity-80 flex-shrink-0">
        KanDev
      </Link>
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
        <Button onClick={onCreateTask} size="lg" className="cursor-pointer">
          <IconPlus className="h-4 w-4" />
          <span className="hidden sm:inline ml-1">Add task</span>
        </Button>
        <TooltipProvider>
          <ViewToggleGroup
            toggleValue={toggleValue}
            onValueChange={handleViewChange}
            className="h-8"
            itemClassName="h-8 w-8"
          />
          {showReleaseNotesButton && <ReleaseNotesButton hasUnseen onClick={onOpenReleaseNotes} />}
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

function DesktopHeader({
  onCreateTask,
  searchQuery,
  onSearchChange,
  isSearchLoading,
  toggleValue,
  handleViewChange,
  showReleaseNotesButton,
  onOpenReleaseNotes,
}: {
  onCreateTask: () => void;
  searchQuery: string;
  onSearchChange?: (query: string) => void;
  isSearchLoading: boolean;
  toggleValue: string;
  handleViewChange: (value: string) => void;
  showReleaseNotesButton: boolean;
  onOpenReleaseNotes: () => void;
}) {
  return (
    <header className="relative flex items-center justify-between p-4 pb-3">
      <div className="flex items-center gap-3">
        <Link href="/" className="text-2xl font-bold hover:opacity-80">
          KanDev
        </Link>
      </div>
      {onSearchChange && (
        <div className="absolute left-1/2 -translate-x-1/2">
          <TaskSearchInput
            value={searchQuery}
            onChange={onSearchChange}
            placeholder="Search tasks..."
            isLoading={isSearchLoading}
          />
        </div>
      )}
      <div className="flex items-center gap-3">
        <Button onClick={onCreateTask} className="cursor-pointer">
          <IconPlus className="h-4 w-4" />
          Add task
        </Button>
        <TooltipProvider>
          <ViewToggleGroup toggleValue={toggleValue} onValueChange={handleViewChange} />
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
        </TooltipProvider>
        <RefreshReviewsButton />
        {showReleaseNotesButton && <ReleaseNotesButton hasUnseen onClick={onOpenReleaseNotes} />}
        <KanbanDisplayDropdown />
        <Link href="/settings" className="cursor-pointer">
          <Button variant="outline" className="cursor-pointer">
            <IconSettings className="h-4 w-4 mr-2" />
            Settings
          </Button>
        </Link>
      </div>
    </header>
  );
}

export function KanbanHeader({
  onCreateTask,
  workspaceId,
  currentPage = "kanban",
  searchQuery = "",
  onSearchChange,
  isSearchLoading = false,
}: KanbanHeaderProps) {
  const router = useRouter();
  const { isMobile, isTablet } = useResponsiveBreakpoint();
  const isMenuOpen = useAppStore((state) => state.mobileKanban.isMenuOpen);
  const setMenuOpen = useAppStore((state) => state.setMobileKanbanMenuOpen);

  const { kanbanViewMode, onViewModeChange } = useKanbanDisplaySettings();
  const releaseNotes = useReleaseNotes();

  const toggleValue = getToggleValue(currentPage, kanbanViewMode);

  const handleViewChange = (value: string) => {
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

  const releaseNotesProps = {
    showReleaseNotesButton: releaseNotes.showTopbarButton,
    onOpenReleaseNotes: releaseNotes.openDialog,
  };

  let header: React.ReactNode;
  if (isMobile) {
    header = (
      <KanbanHeaderMobile
        workspaceId={workspaceId}
        currentPage={currentPage}
        searchQuery={searchQuery}
        onSearchChange={onSearchChange}
        isSearchLoading={isSearchLoading}
        {...releaseNotesProps}
      />
    );
  } else if (isTablet) {
    header = (
      <>
        <TabletHeader
          onCreateTask={onCreateTask}
          searchQuery={searchQuery}
          onSearchChange={onSearchChange}
          isSearchLoading={isSearchLoading}
          toggleValue={toggleValue}
          handleViewChange={handleViewChange}
          setMenuOpen={setMenuOpen}
          {...releaseNotesProps}
        />
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
  } else {
    header = (
      <DesktopHeader
        onCreateTask={onCreateTask}
        searchQuery={searchQuery}
        onSearchChange={onSearchChange}
        isSearchLoading={isSearchLoading}
        toggleValue={toggleValue}
        handleViewChange={handleViewChange}
        {...releaseNotesProps}
      />
    );
  }

  return (
    <>
      {header}
      {releaseNotes.hasNotes && (
        <ReleaseNotesDialog
          open={releaseNotes.dialogOpen}
          onOpenChange={releaseNotes.closeDialog}
          entries={releaseNotes.unseenEntries}
          latestVersion={releaseNotes.latestVersion}
        />
      )}
    </>
  );
}
