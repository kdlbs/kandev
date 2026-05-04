import { useAppStore } from "@/components/state-provider";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";
import type { KanbanState } from "@/lib/state/slices";

type Task = KanbanState["tasks"][number];

/**
 * Read-only lookup of a task by ID across the active workflow and any loaded
 * cross-workflow snapshots. Unlike useTask, this hook does not subscribe to
 * task updates over WebSocket — use it where the caller only needs whatever
 * task data is already cached (e.g. rendering a sender badge for a message
 * that came from another task; if the sender task isn't loaded we fall back
 * to the snapshotted title in metadata).
 */
export function useTaskById(taskId: string | null | undefined): Task | null {
  return useAppStore((state) => {
    if (!taskId) return null;
    const fromActive = state.kanban.tasks.find((item: Task) => item.id === taskId);
    if (fromActive) return fromActive;
    return findTaskInSnapshots(taskId, state.kanbanMulti.snapshots);
  });
}
