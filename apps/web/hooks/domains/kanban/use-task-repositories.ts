"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { multiKanbanQueryOptions } from "@/lib/query/query-options/kanban";
import { useAppStore } from "@/components/state-provider";
import type { KanbanState } from "@/lib/state/slices/kanban/types";

/**
 * Slim per-task repository shape carried in the kanban store. Mirrors
 * KanbanState["tasks"][number]["repositories"][number].
 */
export type KanbanTaskRepository = NonNullable<
  KanbanState["tasks"][number]["repositories"]
>[number];

/**
 * Returns the repositories linked to a task, ordered by Position. Empty
 * array for repo-less tasks.
 *
 * Reads from the TQ kanban multi-cache. Falls back to an empty array when
 * the cache has no data yet or the task is not found.
 *
 * Preserved signature: `useTaskRepositories(taskId)`.
 */
export function useTaskRepositories(taskId: string | null | undefined): KanbanTaskRepository[] {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);

  const { data: multiData } = useQuery({
    ...multiKanbanQueryOptions(workspaceId ?? ""),
    enabled: !!workspaceId,
  });

  return useMemo(() => {
    if (!taskId || !multiData) return [];
    for (const snap of Object.values(multiData.snapshots)) {
      const task = snap.tasks.find((t) => t.id === taskId);
      if (task) {
        const repos = task.repositories ?? [];
        return [...repos].sort((a, b) => a.position - b.position);
      }
    }
    return [];
  }, [taskId, multiData]);
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
