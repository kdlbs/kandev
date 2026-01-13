'use client';

import Link from 'next/link';
import { Button } from '@kandev/ui/button';
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
import { IconAdjustmentsHorizontal, IconPlus, IconSettings } from '@tabler/icons-react';
import { ConnectionStatus } from './connection-status';
import type { Repository } from '@/lib/types/http';

type KanbanBoardHeaderProps = {
  workspaces: Array<{ id: string; name: string }>;
  boards: Array<{ id: string; workspaceId: string; name: string }>;
  activeWorkspaceId: string | null;
  activeBoardId: string | null;
  repositories: Repository[];
  repositoriesLoading: boolean;
  allRepositoriesSelected: boolean;
  selectedRepositoryId: string | null;
  onWorkspaceChange: (workspaceId: string | null) => void;
  onBoardChange: (boardId: string | null) => void;
  onRepositoryChange: (repositoryId: string | 'all') => void;
  onAddTask: () => void;
};

export function KanbanBoardHeader({
  workspaces,
  boards,
  activeWorkspaceId,
  activeBoardId,
  repositories,
  repositoriesLoading,
  allRepositoriesSelected,
  selectedRepositoryId,
  onWorkspaceChange,
  onBoardChange,
  onRepositoryChange,
  onAddTask,
}: KanbanBoardHeaderProps) {
  const repositoryValue = allRepositoriesSelected ? 'all' : selectedRepositoryId ?? 'all';

  return (
    <header className="flex items-center justify-between p-4 pb-3">
      <div className="flex items-center gap-3">
        <Link href="/" className="text-2xl font-bold hover:opacity-80">
          KanDev.ai
        </Link>
        <ConnectionStatus />
      </div>
      <div className="flex items-center gap-3">
        <Button onClick={onAddTask} className="cursor-pointer">
          <IconPlus className="h-4 w-4" />
          Add task
        </Button>
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
            </div>
          </DropdownMenuContent>
        </DropdownMenu>
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
