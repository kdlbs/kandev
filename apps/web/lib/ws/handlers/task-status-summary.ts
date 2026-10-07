import type { AppState } from "@/lib/state/store";
import { isNewerStatusSummary, pickFreshestStatusSummary } from "@/lib/task-status-summary";
import {
  bumpSidebarTaskQueryRevision,
  type KanbanTask,
} from "@/lib/ws/handlers/task-archive-cache";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";

const MAX_SIDEBAR_STATUS_SUMMARIES = 200;
const MAX_SIDEBAR_SUMMARY_WORKSPACES = 8;

type TaskStatusSummaryUpdatedMessage = Parameters<
  NonNullable<WsHandlers["task.status_summary.updated"]>
>[0];

/** Apply a monotonic task.status_summary.updated to kanban + archived caches. */
export function updateTaskStatusSummaryInBothKanbans(
  state: AppState,
  message: TaskStatusSummaryUpdatedMessage,
): AppState {
  const { task_id: taskId, status_summary: nextSummary } = message.payload;
  const previousSummary = freshestCachedTaskSummary(state, taskId);
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

  next = updateTaskStatusSummaryInArchivedCache(next, taskId, shouldReplace, updateTask);
  next = updateSidebarStatusSummary(
    next,
    message.payload.workspace_id,
    taskId,
    pickFreshestStatusSummary(nextSummary, previousSummary) ?? nextSummary,
  );
  return isNewerStatusSummary(nextSummary, previousSummary) &&
    sidebarQuerySummaryChanged(previousSummary, nextSummary)
    ? bumpSidebarTaskQueryRevision(next, message.payload.workspace_id)
    : next;
}

function freshestCachedTaskSummary(state: AppState, taskId: string): TaskStatusSummary | null {
  const summaries: Array<TaskStatusSummary | null | undefined> = [];
  for (const task of state.kanban.tasks) {
    if (task.id === taskId) summaries.push(task.statusSummary);
  }
  for (const snapshot of Object.values(state.kanbanMulti.snapshots)) {
    for (const task of snapshot.tasks) {
      if (task.id === taskId) summaries.push(task.statusSummary);
    }
  }
  for (const tasks of Object.values(state.sidebarArchivedTasks?.itemsByWorkspaceId ?? {})) {
    for (const task of tasks) {
      if (task.id === taskId) summaries.push(task.statusSummary);
    }
  }
  for (const workspaceSummaries of Object.values(state.sidebarStatusSummaryByWorkspaceId ?? {})) {
    summaries.push(workspaceSummaries[taskId]);
  }
  return summaries.reduce<TaskStatusSummary | null>(
    (current, candidate) => pickFreshestStatusSummary(candidate, current) ?? null,
    null,
  );
}

function updateSidebarStatusSummary(
  state: AppState,
  workspaceId: string,
  taskId: string,
  summary: TaskStatusSummary,
): AppState {
  const currentByTaskId = state.sidebarStatusSummaryByWorkspaceId?.[workspaceId] ?? {};
  if (!isNewerStatusSummary(summary, currentByTaskId[taskId])) return state;
  const nextByTaskId = { ...currentByTaskId };
  delete nextByTaskId[taskId];
  nextByTaskId[taskId] = summary;
  const overflow = Object.keys(nextByTaskId).length - MAX_SIDEBAR_STATUS_SUMMARIES;
  if (overflow > 0) {
    for (const oldestTaskId of Object.keys(nextByTaskId).slice(0, overflow)) {
      delete nextByTaskId[oldestTaskId];
    }
  }
  const nextByWorkspace = { ...state.sidebarStatusSummaryByWorkspaceId };
  delete nextByWorkspace[workspaceId];
  nextByWorkspace[workspaceId] = nextByTaskId;
  const oldWorkspaceIds = Object.keys(nextByWorkspace).slice(
    0,
    Math.max(0, Object.keys(nextByWorkspace).length - MAX_SIDEBAR_SUMMARY_WORKSPACES),
  );
  for (const oldestWorkspaceId of oldWorkspaceIds) delete nextByWorkspace[oldestWorkspaceId];
  return {
    ...state,
    sidebarStatusSummaryByWorkspaceId: nextByWorkspace,
  };
}

function sidebarQuerySummaryChanged(
  current: TaskStatusSummary | null | undefined,
  next: TaskStatusSummary,
): boolean {
  return sidebarQuerySummaryKey(current) !== sidebarQuerySummaryKey(next);
}

function sidebarQuerySummaryKey(summary: TaskStatusSummary | null | undefined): string {
  return JSON.stringify([
    summary?.last_activity_at ?? null,
    summary?.primary_session?.state ?? null,
    summary?.has_running_session ?? null,
    hasDiff(summary),
    hasPullRequest(summary),
  ]);
}

function hasDiff(summary: TaskStatusSummary | null | undefined): boolean {
  return (summary?.git?.additions ?? 0) > 0 || (summary?.git?.deletions ?? 0) > 0;
}

function hasPullRequest(summary: TaskStatusSummary | null | undefined): boolean {
  return (summary?.pull_request?.count ?? 0) > 0 || Boolean(summary?.pull_request?.url);
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

  const nextItems = { ...archived.itemsByWorkspaceId };
  for (const workspaceId of changedWorkspaceIds) {
    nextItems[workspaceId] = (nextItems[workspaceId] ?? []).map((task) =>
      task.id === taskId ? updateTask(task) : task,
    );
  }
  return {
    ...state,
    sidebarArchivedTasks: {
      ...archived,
      itemsByWorkspaceId: nextItems,
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
