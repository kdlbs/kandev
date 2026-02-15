'use client';

import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { Button } from '@kandev/ui/button';
import { ToggleGroup, ToggleGroupItem } from '@kandev/ui/toggle-group';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@kandev/ui/tooltip';
import { IconPlus, IconSettings, IconList, IconLayoutKanban, IconMenu2, IconChartBar, IconTimeline } from '@tabler/icons-react';
import { KanbanDisplayDropdown } from '../kanban-display-dropdown';
import { TaskSearchInput } from './task-search-input';
import { KanbanHeaderMobile } from './kanban-header-mobile';
import { MobileMenuSheet } from './mobile-menu-sheet';
import { linkToTasks } from '@/lib/links';
import { useResponsiveBreakpoint } from '@/hooks/use-responsive-breakpoint';
import { useAppStore } from '@/components/state-provider';
import { useKanbanDisplaySettings } from '@/hooks/use-kanban-display-settings';

type KanbanHeaderProps = {
  onCreateTask: () => void;
  workspaceId?: string;
  currentPage?: 'kanban' | 'tasks';
  searchQuery?: string;
  onSearchChange?: (query: string) => void;
  isSearchLoading?: boolean;
};

export function KanbanHeader({ onCreateTask, workspaceId, currentPage = 'kanban', searchQuery = '', onSearchChange, isSearchLoading = false }: KanbanHeaderProps) {
  const router = useRouter();
  const { isMobile, isTablet } = useResponsiveBreakpoint();
  const isMenuOpen = useAppStore((state) => state.mobileKanban.isMenuOpen);
  const setMenuOpen = useAppStore((state) => state.setMobileKanbanMenuOpen);

  const { kanbanViewMode, onViewModeChange } = useKanbanDisplaySettings();

  const toggleValue = currentPage === 'tasks' ? 'list' : (kanbanViewMode === 'graph2' ? 'pipeline' : 'kanban');

  const handleViewChange = (value: string) => {
    if (value === 'list') {
      if (currentPage !== 'tasks') router.push(linkToTasks(workspaceId));
    } else if (value === 'kanban') {
      if (currentPage !== 'kanban') router.push('/');
      onViewModeChange('');
    } else if (value === 'pipeline') {
      if (currentPage !== 'kanban') router.push('/');
      onViewModeChange('graph2');
    }
  };

  // Mobile header: Logo and Hamburger menu (Add task is a FAB)
  if (isMobile) {
    return (
      <KanbanHeaderMobile
        workspaceId={workspaceId}
        currentPage={currentPage}
        searchQuery={searchQuery}
        onSearchChange={onSearchChange}
        isSearchLoading={isSearchLoading}
      />
    );
  }

  // Tablet header: Compact with overflow menu
  if (isTablet) {
    return (
      <>
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
              <ToggleGroup
                type="single"
                value={toggleValue}
                onValueChange={handleViewChange}
                variant="outline"
                className="h-8"
              >
                <ToggleGroupItem
                  value="kanban"
                  className="cursor-pointer h-8 w-8 data-[state=on]:bg-muted data-[state=on]:text-foreground"
                >
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <span className="flex items-center justify-center">
                        <IconLayoutKanban className="h-4 w-4" />
                      </span>
                    </TooltipTrigger>
                    <TooltipContent>Kanban</TooltipContent>
                  </Tooltip>
                </ToggleGroupItem>
                <ToggleGroupItem
                  value="pipeline"
                  className="cursor-pointer h-8 w-8 data-[state=on]:bg-muted data-[state=on]:text-foreground"
                >
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <span className="flex items-center justify-center">
                        <IconTimeline className="h-4 w-4" />
                      </span>
                    </TooltipTrigger>
                    <TooltipContent>Pipeline</TooltipContent>
                  </Tooltip>
                </ToggleGroupItem>
                <ToggleGroupItem
                  value="list"
                  className="cursor-pointer h-8 w-8 data-[state=on]:bg-muted data-[state=on]:text-foreground"
                >
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <span className="flex items-center justify-center">
                        <IconList className="h-4 w-4" />
                      </span>
                    </TooltipTrigger>
                    <TooltipContent>List</TooltipContent>
                  </Tooltip>
                </ToggleGroupItem>
              </ToggleGroup>
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

  // Desktop header: Full layout
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
          <ToggleGroup
            type="single"
            value={toggleValue}
            onValueChange={handleViewChange}
            variant="outline"
          >
            <ToggleGroupItem
              value="kanban"
              className="cursor-pointer data-[state=on]:bg-muted data-[state=on]:text-foreground"
            >
              <Tooltip>
                <TooltipTrigger asChild>
                  <span className="flex items-center justify-center">
                    <IconLayoutKanban className="h-4 w-4" />
                  </span>
                </TooltipTrigger>
                <TooltipContent>Kanban</TooltipContent>
              </Tooltip>
            </ToggleGroupItem>
            <ToggleGroupItem
              value="pipeline"
              className="cursor-pointer data-[state=on]:bg-muted data-[state=on]:text-foreground"
            >
              <Tooltip>
                <TooltipTrigger asChild>
                  <span className="flex items-center justify-center">
                    <IconTimeline className="h-4 w-4" />
                  </span>
                </TooltipTrigger>
                <TooltipContent>Pipeline</TooltipContent>
              </Tooltip>
            </ToggleGroupItem>
            <ToggleGroupItem
              value="list"
              className="cursor-pointer data-[state=on]:bg-muted data-[state=on]:text-foreground"
            >
              <Tooltip>
                <TooltipTrigger asChild>
                  <span className="flex items-center justify-center">
                    <IconList className="h-4 w-4" />
                  </span>
                </TooltipTrigger>
                <TooltipContent>List</TooltipContent>
              </Tooltip>
            </ToggleGroupItem>
          </ToggleGroup>
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
