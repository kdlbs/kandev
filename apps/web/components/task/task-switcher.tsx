"use client";

import { memo, useMemo } from "react";
import type { TaskState } from "@/lib/types/http";
import { truncateRepoPath } from "@/lib/utils";
import { TaskItem } from "./task-item";

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
  isRemoteExecutor?: boolean;
  remoteExecutorType?: string;
  remoteExecutorName?: string;
  updatedAt?: string;
  isArchived?: boolean;
  primarySessionId?: string | null;
};

type TaskSwitcherProps = {
  tasks: TaskSwitcherItem[];
  steps: Array<{ id: string; title: string; color?: string }>;
  activeTaskId: string | null;
  selectedTaskId: string | null;
  onSelectTask: (taskId: string) => void;
  onArchiveTask?: (taskId: string) => void;
  onDeleteTask?: (taskId: string) => void;
  deletingTaskId?: string | null;
  isLoading?: boolean;
};

function TaskSwitcherSkeleton() {
  return (
    <div className="animate-pulse">
      <div className="h-10 bg-foreground/5" />
      <div className="h-10 bg-foreground/5 mt-px" />
      <div className="h-10 bg-foreground/5 mt-px" />
      <div className="h-10 bg-foreground/5 mt-px" />
    </div>
  );
}

export const TaskSwitcher = memo(function TaskSwitcher({
  tasks,
  steps,
  activeTaskId,
  selectedTaskId,
  onSelectTask,
  onArchiveTask,
  onDeleteTask,
  deletingTaskId,
  isLoading = false,
}: TaskSwitcherProps) {
  const stepNameById = useMemo(() => {
    const map = new Map<string, string>();
    for (const col of steps) {
      map.set(col.id, col.title);
    }
    return map;
  }, [steps]);

  // Sort tasks: by step position first, then alphabetically
  const sortedTasks = useMemo(() => {
    const stepOrder = new Map(steps.map((col, i) => [col.id, i]));
    return [...tasks].sort((a, b) => {
      const aOrder = stepOrder.get(a.workflowStepId ?? "") ?? 999;
      const bOrder = stepOrder.get(b.workflowStepId ?? "") ?? 999;
      if (aOrder !== bOrder) return aOrder - bOrder;
      return a.title.localeCompare(b.title);
    });
  }, [tasks, steps]);

  if (isLoading) {
    return <TaskSwitcherSkeleton />;
  }

  return (
    <div>
      {sortedTasks.length === 0 ? (
        <div className="px-3 py-3 text-xs text-muted-foreground">No tasks yet.</div>
      ) : (
        sortedTasks.map((task) => {
          const isActive = task.id === activeTaskId;
          const isSelected = task.id === selectedTaskId || isActive;
          const repoLabel = task.repositoryPath ? truncateRepoPath(task.repositoryPath) : undefined;
          const stepName = task.workflowStepId ? stepNameById.get(task.workflowStepId) : undefined;
          return (
            <TaskItem
              key={task.id}
              title={task.title}
              description={repoLabel}
              stepName={stepName}
              state={task.state}
              isArchived={task.isArchived}
              isSelected={isSelected}
              diffStats={task.diffStats}
              isRemoteExecutor={task.isRemoteExecutor}
              remoteExecutorType={task.remoteExecutorType}
              remoteExecutorName={task.remoteExecutorName}
              taskId={task.id}
              primarySessionId={task.primarySessionId ?? null}
              updatedAt={task.updatedAt}
              onClick={() => onSelectTask(task.id)}
              onArchive={onArchiveTask ? () => onArchiveTask(task.id) : undefined}
              onDelete={onDeleteTask ? () => onDeleteTask(task.id) : undefined}
              isDeleting={deletingTaskId === task.id}
            />
          );
        })
      )}
    </div>
  );
});
