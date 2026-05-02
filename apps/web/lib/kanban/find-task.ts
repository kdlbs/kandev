import type { KanbanState } from "@/lib/state/slices";

type Task = KanbanState["tasks"][number];

// findTaskInSnapshots searches the multi-workflow snapshot map for a task by id.
// Used by callers that need to resolve cross-workflow tasks (e.g. PR-review
// boards, multi-workflow swimlanes) which do not live in `kanban.tasks`.
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
