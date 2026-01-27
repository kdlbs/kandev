'use client';

import { useMemo, useState } from 'react';
import { IconChevronRight } from '@tabler/icons-react';
import type { TaskState } from '@/lib/types/http';
import {
  Item,
  ItemActions,
  ItemContent,
  ItemDescription,
  ItemGroup,
  ItemTitle,
} from '@kandev/ui/item';
import { truncateRepoPath } from '@/lib/utils';
import { TaskStateActions } from './task-state-actions';

type TaskSwitcherItem = {
  id: string;
  title: string;
  state?: TaskState;
  description?: string;
  workflowStepId?: string;
  repositoryPath?: string;
};

type TaskSwitcherProps = {
  tasks: TaskSwitcherItem[];
  columns: Array<{ id: string; title: string; color?: string }>;
  activeTaskId: string | null;
  selectedTaskId: string | null;
  onSelectTask: (taskId: string) => void;
};

export function TaskSwitcher({
  tasks,
  columns,
  activeTaskId,
  selectedTaskId,
  onSelectTask,
}: TaskSwitcherProps) {
  const [collapsedColumnIds, setCollapsedColumnIds] = useState<Set<string>>(() => new Set());

  const sortedTasks = useMemo(() => {
    return [...tasks].sort((a, b) => a.title.localeCompare(b.title));
  }, [tasks]);

  const tasksByColumn = useMemo(() => {
    const grouped: Record<string, TaskSwitcherItem[]> = {};
    for (const task of sortedTasks) {
      const key = task.workflowStepId ?? 'unknown';
      if (!grouped[key]) grouped[key] = [];
      grouped[key].push(task);
    }
    return grouped;
  }, [sortedTasks]);

  const columnsWithFallback = useMemo(() => {
    const seen = new Set(columns.map((column) => column.id));
    const extra: Array<{ id: string; title: string; color?: string }> = [];
    if (tasksByColumn.unknown?.length) {
      extra.push({ id: 'unknown', title: 'Other', color: 'bg-neutral-400' });
    }
    return [...columns, ...extra].filter((column) => {
      if (seen.has(column.id)) return true;
      seen.add(column.id);
      return true;
    });
  }, [columns, tasksByColumn]);

  const handleToggleColumn = (columnId: string) => {
    setCollapsedColumnIds((prev) => {
      const next = new Set(prev);
      if (next.has(columnId)) next.delete(columnId);
      else next.add(columnId);
      return next;
    });
  };

  return (
    <div className="space-y-2">
      <span className="text-xs uppercase tracking-wide text-muted-foreground">Tasks</span>
      {sortedTasks.length === 0 ? (
        <div className="text-xs text-muted-foreground">No tasks on this board.</div>
      ) : (
        <div className="space-y-3">
          {columnsWithFallback.map((column) => {
            const columnTasks = tasksByColumn[column.id] ?? [];
            if (columnTasks.length === 0) return null;
            const isColumnOpen = !collapsedColumnIds.has(column.id);
            return (
              <div key={column.id} className="space-y-2">
                <button
                  type="button"
                  onClick={() => handleToggleColumn(column.id)}
                  className="group flex w-full items-center justify-between cursor-pointer"
                  aria-label="Toggle column"
                >
                  <span className="flex items-center gap-2 truncate text-sm font-medium text-muted-foreground">
                    <span className={`w-2 h-2 rounded-full shrink-0 ${column.color ?? 'bg-neutral-400'}`} />
                    {column.title}
                  </span>
                  <IconChevronRight className={`h-4 w-4 text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity ${isColumnOpen ? 'rotate-90' : ''}`} />
                </button>
                {isColumnOpen && (
                  <div className="flex w-full max-w-md flex-col gap-6">
                    <ItemGroup className="gap-2">
                      {columnTasks.map((task) => {
                        const isActive = task.id === activeTaskId;
                        const isSelected = task.id === selectedTaskId || isActive;
                        const repoLabel = task.repositoryPath ? truncateRepoPath(task.repositoryPath) : 'No repository';
                        return (
                          <Item
                            key={task.id}
                            role="listitem"
                            variant="outline"
                            asChild
                            size="xs"
                            className={isSelected ? 'bg-primary/10 border-primary/30 hover:bg-transparent' : ''}
                          >
                            <a
                              href="#"
                              onClick={(event) => {
                                event.preventDefault();
                                onSelectTask(task.id);
                              }}
                              onKeyDown={(event) => {
                                if (event.key === 'Enter' || event.key === ' ') {
                                  event.preventDefault();
                                  onSelectTask(task.id);
                                }
                              }}
                              className="flex w-full items-center gap-2"
                            >
                              <ItemContent className="min-w-0">
                                <ItemTitle className="text-sm text-foreground">{task.title}</ItemTitle>
                                <ItemDescription className="truncate">{repoLabel}</ItemDescription>
                              </ItemContent>
                              <ItemActions>
                                <TaskStateActions state={task.state} />
                              </ItemActions>
                            </a>
                          </Item>
                        );
                      })}
                    </ItemGroup>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
