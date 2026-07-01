"use client";

import { useMemo } from "react";
import type { KanbanState } from "@/lib/state/slices";
import { useTaskById } from "./use-task-by-id";

/**
 * Slim per-task repository shape carried by task detail data. Mirrors
 * KanbanState["tasks"][number]["repositories"][number].
 */
export type KanbanTaskRepository = NonNullable<
  KanbanState["tasks"][number]["repositories"]
>[number];

/**
 * Returns the repositories linked to a task, ordered by Position. Empty
 * array for repo-less tasks. The hook reads from the task detail Query;
 * use this instead of poking task.repositories directly so multi-repo
 * consumers all share one source of truth.
 */
export function useTaskRepositories(taskId: string | null | undefined): KanbanTaskRepository[] {
  const task = useTaskById(taskId);
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
): KanbanTaskRepository | null {
  const repos = useTaskRepositories(taskId);
  return repos.length > 0 ? repos[0] : null;
}
