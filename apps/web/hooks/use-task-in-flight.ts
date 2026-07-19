import { useAppStore } from "@/components/state-provider";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";
import { isTaskInFlight } from "@/lib/ui/state-icons";
import type { KanbanState } from "@/lib/state/slices";

type Task = KanbanState["tasks"][number];

// useTaskInFlight reports whether a task (single archive/delete) — or ANY task in
// a bulk selection — still has work running, so the archive/delete confirmation
// dialogs can render the §spec:destructive-action-guard "still working" warning.
//
// It resolves the live task straight from the store the same way the board card
// does (active `kanban.tasks` first, then cross-workflow `kanbanMulti.snapshots`,
// mirroring useTask) and applies the shared `isTaskInFlight` aggregate predicate.
// Reading the same source of truth is deliberate: the warning can never disagree
// with the busy indicator the operator already sees on the board.
//
// A bulk delete/archive of several tasks warns when just one is in-flight — the
// guard's job is to stop running work being discarded by accident, so any running
// task in the batch is enough to warn.
export function useTaskInFlight(taskId?: string, taskIds?: string[]): boolean {
  const ids = taskIds ?? (taskId ? [taskId] : []);

  // The selector returns a boolean primitive, so a fresh `ids` array each render
  // is fine — Zustand compares the derived value, not the closure.
  return useAppStore((state) =>
    ids.some((id) => {
      const active = state.kanban.tasks.find((task: Task) => task.id === id);
      const task = active ?? findTaskInSnapshots(id, state.kanbanMulti.snapshots);
      return isTaskInFlight(task?.foregroundActivity);
    }),
  );
}
