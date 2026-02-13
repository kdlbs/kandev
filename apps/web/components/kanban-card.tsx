'use client';

import { useMemo, useState } from 'react';
import { useDraggable } from '@dnd-kit/core';
import { CSS } from '@dnd-kit/utilities';
import { IconDots, IconArrowsMaximize, IconLoader, IconAlertCircle } from '@tabler/icons-react';
import { Card, CardContent } from '@kandev/ui/card';
import { Badge } from '@kandev/ui/badge';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  DropdownMenuSub,
  DropdownMenuSubTrigger,
  DropdownMenuSubContent,
  DropdownMenuPortal,
} from '@kandev/ui/dropdown-menu';
import type { TaskState, Repository } from '@/lib/types/http';
import { cn, getRepositoryDisplayName } from '@/lib/utils';
import { getTaskStateIcon } from '@/lib/ui/state-icons';
import { useAppStore } from '@/components/state-provider';

export interface Task {
  id: string;
  title: string;
  workflowStepId: string;
  state?: TaskState;
  description?: string;
  position?: number;
  repositoryId?: string;
  hasSession?: boolean;
  // Workflow fields
  sessionCount?: number | null;
  primarySessionId?: string | null;
  reviewStatus?: 'pending' | 'approved' | 'changes_requested' | 'rejected' | null;
}

export interface WorkflowColumn {
  id: string;
  title: string;
  color: string;
}

interface KanbanCardProps {
  task: Task;
  repositoryName?: string | null;
  onClick?: (task: Task) => void;
  onEdit?: (task: Task) => void;
  onDelete?: (task: Task) => void;
  onOpenFullPage?: (task: Task) => void;
  onMove?: (task: Task, targetColumnId: string) => void;
  columns?: WorkflowColumn[];
  showMaximizeButton?: boolean;
  isDeleting?: boolean;
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
        <p className="text-xs text-muted-foreground mt-1 leading-tight line-clamp-1">
          {task.description}
        </p>
      )}
      {(task.sessionCount && task.sessionCount > 1 || task.reviewStatus === 'changes_requested' || task.reviewStatus === 'pending') && (
        <div className="flex items-center justify-end gap-2 mt-1">
          {task.sessionCount && task.sessionCount > 1 && (
            <Badge variant="secondary" className="text-xs h-5">
              {task.sessionCount} sessions
            </Badge>
          )}
          {task.reviewStatus === 'pending' && task.state !== 'IN_PROGRESS' && (
            <div className="flex items-center gap-1 text-amber-700 dark:text-amber-600">
              <IconAlertCircle className="h-3.5 w-3.5" />
              <span className="text-[10px] font-medium">Approval Required</span>
            </div>
          )}
          {task.reviewStatus === 'changes_requested' && (
            <Badge variant="outline" className="border-amber-500 text-amber-600 bg-amber-50 dark:bg-amber-950/50 text-xs h-5">
              Changes Requested
            </Badge>
          )}
        </div>
      )}
    </>
  );
}

function KanbanCardLayout({ task, repositoryName, className }: KanbanCardProps & { className?: string }) {
  return (
    <Card size="sm" className={cn('w-full py-0', className)}>
      <CardContent className="px-2 py-1">
        <KanbanCardBody task={task} repoName={repositoryName ?? null} />
      </CardContent>
    </Card>
  );
}

export function KanbanCard({ task, repositoryName, onClick, onEdit, onDelete, onOpenFullPage, onMove, columns, showMaximizeButton = true, isDeleting }: KanbanCardProps) {
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: task.id,
  });
  const [menuOpen, setMenuOpen] = useState(false);

  // Effective open state: keep menu open while deleting
  const effectiveMenuOpen = menuOpen || isDeleting;

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
        'max-h-48 bg-card rounded-sm data-[size=sm]:py-1 cursor-pointer mb-2 w-full py-0 relative border border-border overflow-visible shadow-none ring-0',
        (task.state === 'IN_PROGRESS' || task.state === 'SCHEDULING') && 'kanban-task-pulse',
        isDragging && 'opacity-50 z-50'
      )}
      onClick={() => onClick?.(task)}
      {...listeners}
      {...attributes}
    >
      <CardContent className="px-2 py-1">
        <KanbanCardBody
          task={task}
          repoName={repositoryName ?? null}
          actions={
            <div className="flex items-center gap-2">
              {(task.state === 'IN_PROGRESS' || task.state === 'SCHEDULING') && statusIcon}
              {showMaximizeButton && onOpenFullPage && task.hasSession !== false && (
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
              <DropdownMenu open={effectiveMenuOpen} onOpenChange={(open) => {
                // Prevent closing while deleting
                if (!open && isDeleting) return;
                setMenuOpen(open);
              }}>
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
                    disabled={isDeleting}
                    onClick={(event) => {
                      event.stopPropagation();
                      onEdit?.(task);
                    }}
                  >
                    Edit
                  </DropdownMenuItem>
                  {columns && columns.length > 1 && onMove && (
                    <DropdownMenuSub>
                      <DropdownMenuSubTrigger
                        disabled={isDeleting}
                        onClick={(event) => event.stopPropagation()}
                        onPointerDown={(event) => event.stopPropagation()}
                      >
                        Move to
                      </DropdownMenuSubTrigger>
                      <DropdownMenuPortal>
                        <DropdownMenuSubContent>
                          {columns
                            .filter((col) => col.id !== task.workflowStepId)
                            .map((col) => (
                              <DropdownMenuItem
                                key={col.id}
                                onClick={(event) => {
                                  event.stopPropagation();
                                  onMove(task, col.id);
                                }}
                              >
                                <div className={cn('w-2 h-2 rounded-full mr-2', col.color)} />
                                {col.title}
                              </DropdownMenuItem>
                            ))}
                        </DropdownMenuSubContent>
                      </DropdownMenuPortal>
                    </DropdownMenuSub>
                  )}
                  <DropdownMenuItem
                    disabled={isDeleting}
                    onClick={(event) => {
                      event.stopPropagation();
                      onDelete?.(task);
                    }}
                  >
                    {isDeleting ? <IconLoader className="mr-2 h-4 w-4 animate-spin" /> : null}
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
  // Access store to get repository name for the drag preview
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const repository = useMemo(
    () => (Object.values(repositoriesByWorkspace).flat() as Repository[]).find((repo) => repo.id === task.repositoryId) ?? null,
    [repositoriesByWorkspace, task.repositoryId]
  );
  const repositoryName = repository ? getRepositoryDisplayName(repository.local_path) : null;

  return (
    <KanbanCardLayout
      task={task}
      repositoryName={repositoryName}
      className="cursor-grabbing shadow-lg ring-0 pointer-events-none border border-border"
    />
  );
}
