'use client';

import Link from 'next/link';
import { Button } from '@kandev/ui/button';
import { IconPlus, IconSettings } from '@tabler/icons-react';
import { ConnectionStatus } from './connection-status';
import { KanbanDisplayDropdown } from './kanban-display-dropdown';
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
  enablePreviewOnClick?: boolean;
  onWorkspaceChange: (workspaceId: string | null) => void;
  onBoardChange: (boardId: string | null) => void;
  onRepositoryChange: (repositoryId: string | 'all') => void;
  onTogglePreviewOnClick?: (enabled: boolean) => void;
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
  enablePreviewOnClick,
  onWorkspaceChange,
  onBoardChange,
  onRepositoryChange,
  onTogglePreviewOnClick,
  onAddTask,
}: KanbanBoardHeaderProps) {
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
        <KanbanDisplayDropdown
          workspaces={workspaces}
          boards={boards}
          activeWorkspaceId={activeWorkspaceId}
          activeBoardId={activeBoardId}
          repositories={repositories}
          repositoriesLoading={repositoriesLoading}
          allRepositoriesSelected={allRepositoriesSelected}
          selectedRepositoryId={selectedRepositoryId}
          enablePreviewOnClick={enablePreviewOnClick}
          onWorkspaceChange={onWorkspaceChange}
          onBoardChange={onBoardChange}
          onRepositoryChange={onRepositoryChange}
          onTogglePreviewOnClick={onTogglePreviewOnClick}
        />
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
