'use client';

import Link from 'next/link';
import { useRouter } from 'next/navigation';
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@kandev/ui/sheet';
import { Button } from '@kandev/ui/button';
import { Checkbox } from '@kandev/ui/checkbox';
import { Badge } from '@kandev/ui/badge';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@kandev/ui/select';
import { ToggleGroup, ToggleGroupItem } from '@kandev/ui/toggle-group';
import { IconSettings, IconList, IconLayoutKanban, IconChartBar } from '@tabler/icons-react';
import { TaskSearchInput } from './task-search-input';
import { useKanbanDisplaySettings } from '@/hooks/use-kanban-display-settings';
import { linkToTasks } from '@/lib/links';
import type { Workspace, Repository } from '@/lib/types/http';
import type { WorkflowsState } from '@/lib/state/slices';

type MobileMenuSheetProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId?: string;
  currentPage?: 'kanban' | 'tasks';
  searchQuery?: string;
  onSearchChange?: (query: string) => void;
  isSearchLoading?: boolean;
};

export function MobileMenuSheet({
  open,
  onOpenChange,
  workspaceId,
  currentPage = 'kanban',
  searchQuery = '',
  onSearchChange,
  isSearchLoading = false,
}: MobileMenuSheetProps) {
  const router = useRouter();
  const {
    workspaces,
    workflows,
    activeWorkspaceId,
    activeWorkflowId,
    repositories,
    repositoriesLoading,
    allRepositoriesSelected,
    selectedRepositoryId,
    enablePreviewOnClick,
    onWorkspaceChange,
    onWorkflowChange,
    onRepositoryChange,
    onTogglePreviewOnClick,
  } = useKanbanDisplaySettings();

  const repositoryValue = allRepositoriesSelected ? 'all' : selectedRepositoryId ?? 'all';

  const handleViewChange = (value: string) => {
    if (value === 'list' && currentPage !== 'tasks') {
      router.push(linkToTasks(workspaceId));
      onOpenChange(false);
    } else if (value === 'kanban' && currentPage !== 'kanban') {
      router.push('/');
      onOpenChange(false);
    }
  };

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="w-full sm:max-w-sm overflow-y-auto">
        <SheetHeader>
          <SheetTitle>Menu</SheetTitle>
        </SheetHeader>
        <div className="flex flex-col gap-6 p-4">
          {onSearchChange && (
            <div className="space-y-2">
              <label className="text-sm font-medium">Search</label>
              <TaskSearchInput
                value={searchQuery}
                onChange={onSearchChange}
                placeholder="Search tasks..."
                isLoading={isSearchLoading}
                className="w-full"
              />
            </div>
          )}

          <div className="space-y-2">
            <label className="text-sm font-medium">View</label>
            <ToggleGroup
              type="single"
              value={currentPage === 'tasks' ? 'list' : 'kanban'}
              onValueChange={handleViewChange}
              variant="outline"
              className="w-full justify-start"
            >
              <ToggleGroupItem
                value="kanban"
                className="cursor-pointer flex-1 data-[state=on]:bg-muted data-[state=on]:text-foreground"
              >
                <IconLayoutKanban className="h-4 w-4 mr-2" />
                Kanban
              </ToggleGroupItem>
              <ToggleGroupItem
                value="list"
                className="cursor-pointer flex-1 data-[state=on]:bg-muted data-[state=on]:text-foreground"
              >
                <IconList className="h-4 w-4 mr-2" />
                List
              </ToggleGroupItem>
            </ToggleGroup>
          </div>

          <div className="space-y-4">
            <label className="text-sm font-medium">Display Options</label>

            <div className="space-y-2">
              <label className="text-xs text-muted-foreground">Workspace</label>
              <Select
                value={activeWorkspaceId ?? ''}
                onValueChange={(value) => onWorkspaceChange(value || null)}
              >
                <SelectTrigger className="w-full">
                  <SelectValue placeholder="Select workspace" />
                </SelectTrigger>
                <SelectContent>
                  {workspaces.map((workspace: Workspace) => (
                    <SelectItem key={workspace.id} value={workspace.id}>
                      {workspace.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <label className="text-xs text-muted-foreground">Workflow</label>
              <Select
                value={activeWorkflowId ?? ''}
                onValueChange={(value) => onWorkflowChange(value || null)}
              >
                <SelectTrigger className="w-full">
                  <SelectValue placeholder="Select workflow" />
                </SelectTrigger>
                <SelectContent>
                  {workflows.map((workflow: WorkflowsState['items'][number]) => (
                    <SelectItem key={workflow.id} value={workflow.id}>
                      {workflow.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <label className="text-xs text-muted-foreground">Repository</label>
              <Select
                value={repositoryValue}
                onValueChange={(value) => onRepositoryChange(value as string | 'all')}
                disabled={repositories.length === 0}
              >
                <SelectTrigger className="w-full">
                  <SelectValue
                    placeholder={
                      repositoriesLoading
                        ? 'Loading repositories...'
                        : repositories.length === 0
                          ? 'No repositories'
                          : 'Select repository'
                    }
                  />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All repositories</SelectItem>
                  {repositories.map((repo: Repository) => (
                    <SelectItem key={repo.id} value={repo.id}>
                      {repo.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <label className="text-xs text-muted-foreground">Preview Panel</label>
              <label className="flex items-center gap-2 cursor-pointer">
                <Checkbox
                  checked={enablePreviewOnClick ?? false}
                  onCheckedChange={(checked) => {
                    onTogglePreviewOnClick?.(!!checked);
                  }}
                />
                <span className="text-sm">
                  Open preview on click{' '}
                  <Badge variant="secondary" className="ml-1">beta</Badge>
                </span>
              </label>
            </div>
          </div>

          <div className="mt-auto space-y-2">
            <Link href="/stats" onClick={() => onOpenChange(false)}>
              <Button variant="outline" className="w-full cursor-pointer">
                <IconChartBar className="h-4 w-4 mr-2" />
                Stats
              </Button>
            </Link>
            <Link href="/settings" onClick={() => onOpenChange(false)}>
              <Button variant="outline" className="w-full cursor-pointer">
                <IconSettings className="h-4 w-4 mr-2" />
                Settings
              </Button>
            </Link>
          </div>
        </div>
      </SheetContent>
    </Sheet>
  );
}
