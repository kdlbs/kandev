import type { KanbanState } from "@/lib/state/slices/kanban/types";
import type { AppState } from "@/lib/state/store";
import { toKanbanTask, type TaskLike } from "@/lib/kanban/map-task";

export type KanbanTask = KanbanState["tasks"][number];

export type TaskEventPayload = TaskLike & {
  workspace_id?: string;
  workflow_id: string;
  old_workflow_id?: string | null;
  is_ephemeral?: boolean;
  archived_at?: string | null;
};

export function archivedTaskWorkspaceId(
  state: AppState,
  payload: TaskEventPayload,
): string | undefined {
  if (payload.workspace_id) return payload.workspace_id;
  const taskId = payload.task_id ?? payload.id;
  const cached = Object.entries(state.sidebarArchivedTasks?.itemsByWorkspaceId ?? {}).find(
    ([, tasks]) => tasks.some((task) => task.id === taskId),
  );
  if (cached?.[0]) return cached[0];
  const activeTask = state.kanban.tasks.find((task) => task.id === taskId);
  if (activeTask?.workspaceId) return activeTask.workspaceId;
  for (const snapshot of Object.values(state.kanbanMulti.snapshots)) {
    const snapshotTask = snapshot.tasks.find((task) => task.id === taskId);
    if (snapshotTask?.workspaceId) return snapshotTask.workspaceId;
  }
  return toKanbanTask(payload).workspaceId;
}

export function findArchivedTaskInCache(state: AppState, taskId: string): KanbanTask | undefined {
  for (const tasks of Object.values(state.sidebarArchivedTasks?.itemsByWorkspaceId ?? {})) {
    const task = tasks.find((item) => item.id === taskId);
    if (task) return task;
  }
  return undefined;
}

export function removeTaskFromActiveKanbans(state: AppState, taskId: string): AppState {
  let next = state;
  if (state.kanban.tasks.some((task) => task.id === taskId)) {
    next = {
      ...next,
      kanban: { ...next.kanban, tasks: next.kanban.tasks.filter((task) => task.id !== taskId) },
    };
  }

  const changedSnapshots = Object.entries(next.kanbanMulti.snapshots).filter(([, snapshot]) =>
    snapshot.tasks.some((task) => task.id === taskId),
  );
  if (changedSnapshots.length === 0) return next;

  const nextSnapshots = { ...next.kanbanMulti.snapshots };
  for (const [workflowId, snapshot] of changedSnapshots) {
    nextSnapshots[workflowId] = {
      ...snapshot,
      tasks: snapshot.tasks.filter((task) => task.id !== taskId),
    };
  }
  return {
    ...next,
    kanbanMulti: { ...next.kanbanMulti, snapshots: nextSnapshots },
  };
}

/** Remove a task from both single-kanban and multi-kanban snapshots. */
export function removeTaskFromBothKanbans(state: AppState, taskId: string): AppState {
  const next = removeTaskFromActiveKanbans(state, taskId);
  const archivedItems = next.sidebarArchivedTasks?.itemsByWorkspaceId;
  if (!archivedItems) return next;

  const changedWorkspaceIds = Object.entries(archivedItems)
    .filter(([, tasks]) => tasks.some((task) => task.id === taskId))
    .map(([workspaceId]) => workspaceId);
  if (changedWorkspaceIds.length === 0) return next;

  const revisions = next.sidebarArchivedTasks.revisionByWorkspaceId ?? {};
  return {
    ...next,
    sidebarArchivedTasks: {
      ...next.sidebarArchivedTasks,
      itemsByWorkspaceId: Object.fromEntries(
        Object.entries(archivedItems).map(([workspaceId, tasks]) => [
          workspaceId,
          tasks.filter((task) => task.id !== taskId),
        ]),
      ),
      revisionByWorkspaceId: {
        ...revisions,
        ...Object.fromEntries(
          changedWorkspaceIds.map((workspaceId) => [
            workspaceId,
            (revisions[workspaceId] ?? 0) + 1,
          ]),
        ),
      },
    },
  };
}
