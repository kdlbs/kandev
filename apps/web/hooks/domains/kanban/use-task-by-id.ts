import { useQuery } from "@tanstack/react-query";
import { toKanbanTask } from "@/lib/kanban/map-task";
import { taskQueryOptions } from "@/lib/query/query-options";
import type { KanbanState } from "@/lib/state/slices";

type Task = KanbanState["tasks"][number];

/**
 * Read-only lookup of a task by ID from TanStack Query. Unlike useTask, this
 * hook does not subscribe to task updates over WebSocket — use it where the
 * caller only needs fetched/cached task data.
 */
export function useTaskById(taskId: string | null | undefined): Task | null {
  const query = useQuery({
    ...taskQueryOptions(taskId ?? ""),
    enabled: Boolean(taskId),
  });

  return query.data ? toKanbanTask(query.data) : null;
}
