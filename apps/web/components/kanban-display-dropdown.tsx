'use client';

import { Button } from '@kandev/ui/button';
import { Checkbox } from '@kandev/ui/checkbox';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@kandev/ui/dropdown-menu';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@kandev/ui/select';
import { IconAdjustmentsHorizontal } from '@tabler/icons-react';
import type { Repository } from '@/lib/types/http';

type KanbanDisplayDropdownProps = {
  workspaces: Array<{ id: string; name: string }>;
  boards: Array<{ id: string; workspaceId: string; name: string }>;
  activeWorkspaceId: string | null;
  activeBoardId: string | null;
  repositories: Repository[];
  repositoriesLoading: boolean;
  allRepositoriesSelected: boolean;
  selectedRepositoryId: string | null;
  enablePreviewOnClick?: boolean;
  onWorkspaceChange: (workspaceId: string | null) => void;
  onBoardChange: (boardId: string | null) => void;
  onRepositoryChange: (repositoryId: string | 'all') => void;
  onTogglePreviewOnClick?: (enabled: boolean) => void;
};

export function KanbanDisplayDropdown({
  workspaces,
  boards,
  activeWorkspaceId,
  activeBoardId,
  repositories,
  repositoriesLoading,
  allRepositoriesSelected,
  selectedRepositoryId,
  enablePreviewOnClick,
  onWorkspaceChange,
  onBoardChange,
  onRepositoryChange,
  onTogglePreviewOnClick,
}: KanbanDisplayDropdownProps) {
  const repositoryValue = allRepositoriesSelected ? 'all' : selectedRepositoryId ?? 'all';

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline" className="cursor-pointer">
          <IconAdjustmentsHorizontal className="h-4 w-4 mr-2" />
          Display
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-[280px] p-3">
        <div className="space-y-3">
          <div className="space-y-1.5">
            <DropdownMenuLabel className="px-0">Workspace</DropdownMenuLabel>
            <Select
              value={activeWorkspaceId ?? ''}
              onValueChange={(value) => onWorkspaceChange(value || null)}
            >
              <SelectTrigger className="w-full">
                <SelectValue placeholder="Select workspace" />
              </SelectTrigger>
              <SelectContent>
                {workspaces.map((workspace) => (
                  <SelectItem key={workspace.id} value={workspace.id}>
                    {workspace.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <DropdownMenuSeparator />
          <div className="space-y-1.5">
            <DropdownMenuLabel className="px-0">Board</DropdownMenuLabel>
            <Select
              value={activeBoardId ?? ''}
              onValueChange={(value) => onBoardChange(value || null)}
            >
              <SelectTrigger className="w-full">
                <SelectValue placeholder="Select board" />
              </SelectTrigger>
              <SelectContent>
                {boards.map((board) => (
                  <SelectItem key={board.id} value={board.id}>
                    {board.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <DropdownMenuSeparator />
          <div className="space-y-1.5">
            <DropdownMenuLabel className="px-0">Repository</DropdownMenuLabel>
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
                {repositories.map((repo) => (
                  <SelectItem key={repo.id} value={repo.id}>
                    {repo.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <DropdownMenuSeparator />
          <div className="space-y-1.5">
            <DropdownMenuLabel className="px-0">Preview Panel</DropdownMenuLabel>
            <label className="flex items-center gap-2 cursor-pointer">
              <Checkbox
                checked={enablePreviewOnClick ?? false}
                onCheckedChange={(checked) => {
                  onTogglePreviewOnClick?.(!!checked);
                }}
              />
              <span className="text-sm">Open preview on click</span>
            </label>
            <p className="text-xs text-muted-foreground pl-6">
              When enabled, clicking a task opens the preview panel. When disabled, clicking navigates directly to the session.
            </p>
          </div>
        </div>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
