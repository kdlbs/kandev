import type { KanbanState } from "@/lib/state/slices";

type Task = KanbanState["tasks"][number];

// `fallbackTasks` is consulted when no snapshot matches (e.g. WS-arrived tasks before their snapshot lands).
export function findTaskInSnapshots(
  taskId: string,
  snapshots: Record<string, { tasks: KanbanState["tasks"] }>,
  fallbackTasks?: KanbanState["tasks"],
): Task | null {
  for (const snapshot of Object.values(snapshots)) {
    const found = snapshot.tasks.find((t) => t.id === taskId);
    if (found) return found;
  }
  return fallbackTasks?.find((t) => t.id === taskId) ?? null;
}
