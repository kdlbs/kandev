'use client';

import { memo, useMemo, useState } from 'react';
import type { TaskState } from '@/lib/types/http';
import { truncateRepoPath } from '@/lib/utils';
import { SidebarButton } from './sidebar-button';
import { TaskItem } from './task-item';

type DiffStats = {
  additions: number;
  deletions: number;
};

type TaskSwitcherItem = {
  id: string;
  title: string;
  state?: TaskState;
  description?: string;
  workflowStepId?: string;
  repositoryPath?: string;
  diffStats?: DiffStats;
  updatedAt?: string;
};

type TaskSwitcherProps = {
  tasks: TaskSwitcherItem[];
  columns: Array<{ id: string; title: string; color?: string }>;
  activeTaskId: string | null;
  selectedTaskId: string | null;
  onSelectTask: (taskId: string) => void;
  onDeleteTask?: (taskId: string) => void;
  deletingTaskId?: string | null;
  isLoading?: boolean;
};

function TaskSwitcherSkeleton() {
  return (
    <div className="space-y-3 animate-pulse">
      {/* Column skeleton */}
      <div className="space-y-1">
        <div className="h-7 bg-foreground/5 rounded-md" />
        <div className="space-y-1 pl-0">
          <div className="h-14 bg-foreground/5 rounded-lg" />
          <div className="h-14 bg-foreground/5 rounded-lg" />
        </div>
      </div>
      {/* Another column skeleton */}
      <div className="space-y-1">
        <div className="h-7 bg-foreground/5 rounded-md" />
        <div className="space-y-1 pl-0">
          <div className="h-14 bg-foreground/5 rounded-lg" />
        </div>
      </div>
    </div>
  );
}

export const TaskSwitcher = memo(function TaskSwitcher({
  tasks,
  columns,
  activeTaskId,
  selectedTaskId,
  onSelectTask,
  onDeleteTask,
  deletingTaskId,
  isLoading = false,
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

  if (isLoading) {
    return <TaskSwitcherSkeleton />;
  }

  return (
    <div className="space-y-1">
      {sortedTasks.length === 0 ? (
        <div className="px-2 text-xs text-muted-foreground">No tasks on this board.</div>
      ) : (
        <div className="space-y-1">
          {columnsWithFallback.map((column) => {
            const columnTasks = tasksByColumn[column.id] ?? [];
            if (columnTasks.length === 0) return null;
            const isColumnOpen = !collapsedColumnIds.has(column.id);
            return (
              <div key={column.id}>
                <SidebarButton
                  label={column.title}
                  count={columnTasks.length}
                  isExpanded={isColumnOpen}
                  onToggle={() => handleToggleColumn(column.id)}
                />
                {/* Expanded content with CSS transition */}
                <div
                  className={`overflow-hidden transition-[opacity,grid-template-rows] duration-200 ease-in-out ${isColumnOpen
                      ? 'grid grid-rows-[1fr] opacity-100'
                      : 'grid grid-rows-[0fr] opacity-0'
                    }`}
                >
                  <div className="min-h-0">
                    <div className="pt-1">
                      <div className="flex flex-col gap-1">
                        {columnTasks.map((task) => {
                          const isActive = task.id === activeTaskId;
                          const isSelected = task.id === selectedTaskId || isActive;
                          const repoLabel = task.repositoryPath
                            ? truncateRepoPath(task.repositoryPath)
                            : undefined;
                          return (
                            <TaskItem
                              key={task.id}
                              title={task.title}
                              description={repoLabel}
                              state={task.state}
                              isSelected={isSelected}
                              diffStats={task.diffStats}
                              updatedAt={task.updatedAt}
                              onClick={() => onSelectTask(task.id)}
                              onDelete={onDeleteTask ? () => onDeleteTask(task.id) : undefined}
                              isDeleting={deletingTaskId === task.id}
                            />
                          );
                        })}
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
});
