import type { KanbanState } from "@/lib/state/slices";

type Task = KanbanState["tasks"][number];

export function findTaskInSnapshots(
  taskId: string,
  snapshots: Record<string, { tasks: KanbanState["tasks"] }>,
): Task | null {
  for (const snapshot of Object.values(snapshots)) {
    const found = snapshot.tasks.find((t) => t.id === taskId);
    if (found) return found;
  }
  return null;
}
