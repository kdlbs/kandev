"use client";

import { useQuery } from "@tanstack/react-query";
import { multiKanbanQueryOptions } from "@/lib/query/query-options/kanban";
import { useAppStore } from "@/components/state-provider";

/**
 * Repo-status of the active task, distinguishing three states:
 *   - `null`   — unknown (no active task, or task hasn't loaded into the
 *                cache yet); callers must not act on this.
 *   - `true`   — task has at least one repository.
 *   - `false`  — task is confirmed repo-less.
 *
 * Returning a boolean would conflate "loading" with "no repos", which would
 * cause auto-close effects (e.g. the Changes panel) to tear panels down on
 * first render before the task data arrives — `removePanel` is permanent
 * within a session, so the user couldn't recover them.
 *
 * Reads `qk.kanban.multi()` directly so we can distinguish "query result
 * not yet returned" (`multiData === undefined`) from "task absent from a
 * loaded snapshot" — the wrapped `useTaskRepositories` hook collapses both
 * into `[]`, which previously caused a /t/:id direct navigation (without
 * first visiting kanban) to flag the active task as repo-less and remove
 * the Changes panel.
 */
export function useActiveTaskHasRepos(): boolean | null {
  const taskId = useAppStore((s) => s.tasks.activeTaskId);
  const workspaceId = useAppStore((s) => s.workspaces.activeId);

  const { data: multiData } = useQuery({
    ...multiKanbanQueryOptions(workspaceId ?? ""),
    enabled: !!workspaceId,
  });

  if (!taskId) return null;
  if (!multiData) return null; // cache still loading

  for (const snap of Object.values(multiData.snapshots)) {
    const task = snap.tasks.find((t) => t.id === taskId);
    if (task) return (task.repositories ?? []).length > 0;
  }

  // Task not in any snapshot — could be a fresh task whose snapshot hasn't
  // been refreshed, or a task in a workflow not yet visited. Treat as
  // unknown so we don't accidentally remove the Changes panel.
  return null;
}
