'use client';

import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { Button } from '@kandev/ui/button';
import { ToggleGroup, ToggleGroupItem } from '@kandev/ui/toggle-group';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@kandev/ui/tooltip';
import { IconPlus, IconSettings, IconList, IconLayoutKanban } from '@tabler/icons-react';
import { ConnectionStatus } from '../connection-status';
import { KanbanDisplayDropdown } from '../kanban-display-dropdown';
import { TaskSearchInput } from './task-search-input';
import { linkToTasks } from '@/lib/links';

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

  const handleViewChange = (value: string) => {
    if (value === 'list' && currentPage !== 'tasks') {
      router.push(linkToTasks(workspaceId));
    } else if (value === 'kanban' && currentPage !== 'kanban') {
      router.push('/');
    }
  };

  return (
    <header className="relative flex items-center justify-between p-4 pb-3">
      <div className="flex items-center gap-3">
        <Link href="/" className="text-2xl font-bold hover:opacity-80">
          KanDev.ai
        </Link>
        <ConnectionStatus />
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
            value={currentPage === 'tasks' ? 'list' : 'kanban'}
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
