import type { AppState } from "@/lib/state/store";
import type { Task } from "@/lib/types/http";
import type { TaskOverview } from "@/lib/state/slices/task-overview-types";
import { toKanbanTask } from "@/lib/kanban/map-task";
import { reconcileTaskOverviewRead } from "@/lib/state/slices/task-overview-merge";

export function acceptWorkflowSnapshotOverview(
  state: AppState,
  tasks: TaskOverview[],
  readId: string | undefined,
  workflowId: string,
  original: Task[],
): TaskOverview[] | null {
  const incoming = new Map(
    original.map((task) => {
      const row = toKanbanTask(task);
      return [row.id, row];
    }),
  );
  const scoped = tasks.map((task) => ({
    ...task,
    workflowId: task.workflowId ?? workflowId,
    // Lifecycle backfills must not make a stale HTTP row fresh stage evidence.
    ...(state.taskOverview && incoming.has(task.id)
      ? { updatedAt: incoming.get(task.id)!.updatedAt }
      : {}),
  }));
  const reconciled = state.taskOverview
    ? reconcileTaskOverviewRead(state.taskOverview, scoped, readId, true)
    : scoped;
  if (!reconciled) return null;
  const members = reconciled.filter((task) => task.workflowId === workflowId && !task.isArchived);
  if (readId) {
    const admittedIds = new Set(members.map((task) => task.id));
    state.finishTaskOverviewRead(
      readId,
      [...incoming.values()].filter((task) => admittedIds.has(task.id)),
    );
  }
  return members;
}
