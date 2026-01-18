'use client';

import { useDraggable } from '@dnd-kit/core';
import { CSS } from '@dnd-kit/utilities';
import { IconDots, IconArrowsMaximize } from '@tabler/icons-react';
import { Card, CardContent } from '@kandev/ui/card';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@kandev/ui/dropdown-menu';
import { useMemo } from 'react';
import { useAppStore } from '@/components/state-provider';
import type { TaskState } from '@/lib/types/http';
import { cn, getRepositoryDisplayName } from '@/lib/utils';
import { getTaskStateIcon } from '@/lib/ui/state-icons';

export interface Task {
  id: string;
  title: string;
  columnId: string;
  state?: TaskState;
  description?: string;
  position?: number;
  repositoryId?: string;
  hasSession?: boolean;
}

interface KanbanCardProps {
  task: Task;
  onClick?: (task: Task) => void;
  onEdit?: (task: Task) => void;
  onDelete?: (task: Task) => void;
  onOpenFullPage?: (task: Task) => void;
}

function KanbanCardBody({
  task,
  repoName,
  actions,
}: {
  task: Task;
  repoName: string | null;
  actions?: React.ReactNode;
}) {
  return (
    <>
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0 flex-1">
          {repoName && (
            <p className="text-[10px] mb-1 text-muted-foreground leading-tight truncate">{repoName}</p>
          )}
          <p className="text-sm font-medium leading-tight line-clamp-1">{task.title}</p>
        </div>
        {actions}
      </div>
      {task.description && (
        <p className="text-xs text-muted-foreground mt-1 leading-tight line-clamp-3">
          {task.description}
        </p>
      )}
    </>
  );
}

function KanbanCardLayout({ task, className }: KanbanCardProps & { className?: string }) {
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const repository = useMemo(
    () => Object.values(repositoriesByWorkspace).flat().find((repo) => repo.id === task.repositoryId) ?? null,
    [repositoriesByWorkspace, task.repositoryId]
  );
  const repoName = getRepositoryDisplayName(repository?.local_path);

  return (
    <Card size="sm" className={cn('w-full py-0', className)}>
      <CardContent className="px-2 py-1">
        <KanbanCardBody task={task} repoName={repoName} />
      </CardContent>
    </Card>
  );
}

export function KanbanCard({ task, onClick, onEdit, onDelete, onOpenFullPage }: KanbanCardProps) {
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: task.id,
  });

  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const repository = useMemo(
    () => Object.values(repositoriesByWorkspace).flat().find((repo) => repo.id === task.repositoryId) ?? null,
    [repositoriesByWorkspace, task.repositoryId]
  );
  const repoName = getRepositoryDisplayName(repository?.local_path);

  const statusIcon = getTaskStateIcon(task.state, 'h-4 w-4');

  const style = {
    transform: CSS.Translate.toString(transform),
    transition: 'none',
    willChange: isDragging ? 'transform' : undefined,
  };

  return (
    <Card
      size="sm"
      ref={setNodeRef}
      style={style}
      className={cn(
        'max-h-48 rounded-sm data-[size=sm]:py-1 cursor-pointer mb-2 w-full py-0 relative border border-border overflow-visible shadow-none ring-0',
        task.state === 'IN_PROGRESS' && 'kanban-task-pulse',
        isDragging && 'opacity-50 z-50'
      )}
      onClick={() => onClick?.(task)}
      {...listeners}
      {...attributes}
    >
      <CardContent className="px-2 py-1">
        <KanbanCardBody
          task={task}
          repoName={repoName}
          actions={
            <div className="flex items-center gap-2">
              {statusIcon}
              {onOpenFullPage && task.hasSession !== false && (
                <button
                  type="button"
                  className="text-muted-foreground hover:text-foreground hover:bg-accent rounded-sm p-1 -m-1 transition-colors cursor-pointer"
                  onClick={(event) => {
                    event.stopPropagation();
                    onOpenFullPage(task);
                  }}
                  onPointerDown={(event) => event.stopPropagation()}
                  aria-label="Open full page"
                  title="Open full page"
                >
                  <IconArrowsMaximize className="h-4 w-4" />
                </button>
              )}
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <button
                    type="button"
                    className="text-muted-foreground hover:text-foreground hover:bg-muted rounded-sm p-1 -m-1 transition-colors cursor-pointer"
                    onClick={(event) => event.stopPropagation()}
                    onPointerDown={(event) => event.stopPropagation()}
                    aria-label="More options"
                  >
                    <IconDots className="h-4 w-4" />
                  </button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem
                    onClick={(event) => {
                      event.stopPropagation();
                      onEdit?.(task);
                    }}
                  >
                    Edit
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onClick={(event) => {
                      event.stopPropagation();
                      onDelete?.(task);
                    }}
                  >
                    Delete
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          }
        />
      </CardContent>
    </Card>
  );
}

export function KanbanCardPreview({ task }: KanbanCardProps) {
  return (
    <KanbanCardLayout
      task={task}
      className="cursor-grabbing shadow-lg ring-0 pointer-events-none border border-border"
    />
  );
}
