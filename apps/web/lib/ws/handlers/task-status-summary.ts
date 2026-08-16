import type { AppState } from "@/lib/state/store";
import { isNewerStatusSummary } from "@/lib/task-status-summary";
import type { KanbanTask } from "@/lib/ws/handlers/task-archive-cache";
import type { WsHandlers } from "@/lib/ws/handlers/types";

type TaskStatusSummaryUpdatedMessage = Parameters<
  NonNullable<WsHandlers["task.status_summary.updated"]>
>[0];

/** Apply a monotonic task.status_summary.updated to kanban + archived caches. */
export function updateTaskStatusSummaryInBothKanbans(
  state: AppState,
  message: TaskStatusSummaryUpdatedMessage,
): AppState {
  const { task_id: taskId, status_summary: nextSummary } = message.payload;
  const shouldReplace = (task: KanbanTask): boolean =>
    isNewerStatusSummary(nextSummary, task.statusSummary);
  const updateTask = (task: KanbanTask): KanbanTask =>
    shouldReplace(task) ? { ...task, statusSummary: nextSummary } : task;

  let next = state;
  if (state.kanban.tasks.some((task) => task.id === taskId && shouldReplace(task))) {
    next = {
      ...next,
      kanban: {
        ...next.kanban,
        tasks: next.kanban.tasks.map((task) => (task.id === taskId ? updateTask(task) : task)),
      },
    };
  }

  const snapshots = Object.entries(next.kanbanMulti.snapshots);
  const changedSnapshots = snapshots.filter(([, snapshot]) =>
    snapshot.tasks.some((task) => task.id === taskId && shouldReplace(task)),
  );
  if (changedSnapshots.length > 0) {
    const nextSnapshots = { ...next.kanbanMulti.snapshots };
    for (const [workflowId, snapshot] of changedSnapshots) {
      nextSnapshots[workflowId] = {
        ...snapshot,
        tasks: snapshot.tasks.map((task) => (task.id === taskId ? updateTask(task) : task)),
      };
    }
    next = {
      ...next,
      kanbanMulti: { ...next.kanbanMulti, snapshots: nextSnapshots },
    };
  }

  return updateTaskStatusSummaryInArchivedCache(next, taskId, shouldReplace, updateTask);
}

function updateTaskStatusSummaryInArchivedCache(
  state: AppState,
  taskId: string,
  shouldReplace: (task: KanbanTask) => boolean,
  updateTask: (task: KanbanTask) => KanbanTask,
): AppState {
  const archived = state.sidebarArchivedTasks;
  if (!archived?.itemsByWorkspaceId) return state;

  const changedWorkspaceIds = Object.entries(archived.itemsByWorkspaceId)
    .filter(([, tasks]) => tasks.some((task) => task.id === taskId && shouldReplace(task)))
    .map(([workspaceId]) => workspaceId);
  if (changedWorkspaceIds.length === 0) return state;

  const revisions = archived.revisionByWorkspaceId ?? {};
  const nextItems = { ...archived.itemsByWorkspaceId };
  const nextRevisions = { ...revisions };
  for (const workspaceId of changedWorkspaceIds) {
    nextItems[workspaceId] = (nextItems[workspaceId] ?? []).map((task) =>
      task.id === taskId ? updateTask(task) : task,
    );
    nextRevisions[workspaceId] = (revisions[workspaceId] ?? 0) + 1;
  }
  return {
    ...state,
    sidebarArchivedTasks: {
      ...archived,
      itemsByWorkspaceId: nextItems,
      revisionByWorkspaceId: nextRevisions,
    },
  };
}

export function removeArchivedTaskFromCache(state: AppState, taskId: string): AppState {
  if (!state.sidebarArchivedTasks) return state;
  const revisions = state.sidebarArchivedTasks.revisionByWorkspaceId ?? {};
  const changedWorkspaceIds = Object.entries(state.sidebarArchivedTasks.itemsByWorkspaceId)
    .filter(([, tasks]) => tasks.some((task) => task.id === taskId))
    .map(([workspaceId]) => workspaceId);
  if (changedWorkspaceIds.length === 0) return state;
  return {
    ...state,
    sidebarArchivedTasks: {
      ...state.sidebarArchivedTasks,
      itemsByWorkspaceId: Object.fromEntries(
        Object.entries(state.sidebarArchivedTasks.itemsByWorkspaceId).map(
          ([workspaceId, tasks]) => [workspaceId, tasks.filter((task) => task.id !== taskId)],
        ),
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
