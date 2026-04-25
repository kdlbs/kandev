"use client";

import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import type { KanbanState } from "@/lib/state/slices";
import { primaryTaskRepository, type TaskRepository } from "@/lib/types/http";

/**
 * Returns the repositories linked to a task, ordered by Position. Empty
 * array for repo-less tasks. The hook reads from the kanban tasks slice;
 * use this instead of poking task.repositories directly so multi-repo
 * consumers all share one source of truth.
 */
export function useTaskRepositories(taskId: string | null | undefined): TaskRepository[] {
  const task = useAppStore((state) =>
    taskId
      ? (state.kanban.tasks.find((t: KanbanState["tasks"][number]) => t.id === taskId) ?? null)
      : null,
  );
  return useMemo(() => {
    const repos = task?.repositories ?? [];
    return [...repos].sort((a, b) => a.position - b.position);
  }, [task]);
}

/**
 * Returns the primary repository for a task (lowest position), or null.
 */
export function useTaskPrimaryRepository(
  taskId: string | null | undefined,
): TaskRepository | null {
  const repos = useTaskRepositories(taskId);
  return primaryTaskRepository(repos) ?? null;
}
