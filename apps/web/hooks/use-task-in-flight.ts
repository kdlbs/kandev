import { useAppStore } from "@/components/state-provider";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";
import { isTaskInFlight } from "@/lib/ui/state-icons";
import type { KanbanState } from "@/lib/state/slices";

type Task = KanbanState["tasks"][number];

// Resolve the same active-or-snapshot task records used by board indicators.
// Bulk actions warn when any selected task is still working.
export function useTaskInFlight(taskId?: string, taskIds?: string[], enabled = true): boolean {
  const ids = enabled ? (taskIds ?? (taskId ? [taskId] : [])) : [];

  return useAppStore((state) =>
    ids.some((id) => {
      const active = state.kanban.tasks.find((task: Task) => task.id === id);
      const task = active ?? findTaskInSnapshots(id, state.kanbanMulti.snapshots);
      return isTaskInFlight(task?.foregroundActivity);
    }),
  );
}
