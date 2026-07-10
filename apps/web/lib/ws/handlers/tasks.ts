import type { StoreApi } from "zustand";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import { cleanupTaskStorage } from "@/lib/local-storage";
import { removeRecentTask } from "@/lib/recent-tasks";
import { useContextFilesStore } from "@/lib/state/context-files-store";
import { toKanbanTask, type TaskLike } from "@/lib/kanban/map-task";
import { sessionId as toSessionId } from "@/lib/types/http";
import { mergeTaskRepositoryFields } from "@/lib/ws/handlers/task-repositories";
import { softNavigate } from "@/lib/routing/client-router";
import { isTaskDetailPath, normalizePathname } from "@/lib/links";
import {
  clearPinnedSessionIfOverridden,
  shouldPreservePinnedSessionForTask,
} from "@/lib/ws/handlers/agent-session";

type KanbanTask = KanbanState["tasks"][number];
const lifecycleDebug = createDebugLogger("task-lifecycle:ws");

function hasPayloadField(payload: TaskEventPayload, field: keyof TaskEventPayload): boolean {
  return Object.prototype.hasOwnProperty.call(payload, field);
}

function mergeTaskUpdate(
  existing: KanbanTask | undefined,
  nextTask: KanbanTask,
  payload: TaskEventPayload,
): KanbanTask {
  if (!existing) return nextTask;
  const merged = {
    ...nextTask,
    ...mergeTaskRepositoryFields(existing, nextTask),
  };
  if (!hasPayloadField(payload, "primary_session_id") && nextTask.primarySessionId === undefined) {
    merged.primarySessionId = existing.primarySessionId;
  }
  if (
    !hasPayloadField(payload, "primary_session_state") &&
    nextTask.primarySessionState === undefined
  ) {
    merged.primarySessionState = existing.primarySessionState;
  }
  if (
    !hasPayloadField(payload, "primary_session_pending_action") &&
    nextTask.primarySessionPendingAction === undefined
  ) {
    merged.primarySessionPendingAction = existing.primarySessionPendingAction;
  }
  return merged;
}

function upsertTask(
  tasks: KanbanTask[],
  nextTask: KanbanTask,
  payload: TaskEventPayload,
): KanbanTask[] {
  const existing = tasks.find((task) => task.id === nextTask.id);
  const merged = mergeTaskUpdate(existing, nextTask, payload);
  return existing
    ? tasks.map((task) => (task.id === nextTask.id ? merged : task))
    : [...tasks, merged];
}

function upsertMultiTask(
  state: AppState,
  workflowId: string,
  task: KanbanTask,
  payload: TaskEventPayload,
): AppState {
  const snapshot = state.kanbanMulti.snapshots[workflowId];
  if (!snapshot) {
    const workflowName =
      state.workflows?.items.find((item) => item.id === workflowId)?.name ?? workflowId;
    return {
      ...state,
      kanbanMulti: {
        ...state.kanbanMulti,
        snapshots: {
          ...state.kanbanMulti.snapshots,
          [workflowId]: {
            workflowId,
            workflowName,
            steps: [],
            tasks: upsertTask([], task, payload),
            isPlaceholder: true,
          },
        },
      },
    };
  }
  return {
    ...state,
    kanbanMulti: {
      ...state.kanbanMulti,
      snapshots: {
        ...state.kanbanMulti.snapshots,
        [workflowId]: {
          ...snapshot,
          tasks: upsertTask(snapshot.tasks, task, payload),
        },
      },
    },
  };
}

type TaskEventPayload = TaskLike & {
  workflow_id: string;
  old_workflow_id?: string | null;
  is_ephemeral?: boolean;
  archived_at?: string | null;
};

/** Upsert a task in both single-kanban and multi-kanban snapshots. */
function upsertTaskInBothKanbans(
  state: AppState,
  wfId: string,
  payload: TaskEventPayload,
): AppState {
  // Skip ephemeral tasks - they should never be added to kanban
  if (payload.is_ephemeral) {
    return state;
  }

  const nextTask = toKanbanTask(payload);
  let next = state;

  if (state.kanban.workflowId === wfId) {
    next = {
      ...next,
      kanban: { ...next.kanban, tasks: upsertTask(next.kanban.tasks, nextTask, payload) },
    };
  }

  next = upsertMultiTask(next, wfId, nextTask, payload);

  return next;
}

/** Look up a task across both single-kanban and multi-kanban snapshots. */
function findTaskInState(state: AppState, taskId: string): KanbanTask | undefined {
  const fromKanban = state.kanban.tasks.find((t) => t.id === taskId);
  if (fromKanban) return fromKanban;
  for (const snapshot of Object.values(state.kanbanMulti.snapshots)) {
    const found = snapshot.tasks.find((t) => t.id === taskId);
    if (found) return found;
  }
  return undefined;
}

function taskEventIdForLog(payload: TaskEventPayload): string {
  return payload.task_id ?? payload.id ?? "";
}

function valueForLog(value: string | null | undefined): string {
  return value ?? "-";
}

function payloadPrimaryStateForLog(payload: TaskEventPayload): string {
  if (payload.primary_session_state === undefined) return "-";
  return payload.primary_session_state ?? "null";
}

function taskStateForLog(task: KanbanTask | undefined): string {
  return task?.state ?? "-";
}

function taskPrimaryStateForLog(task: KanbanTask | undefined): string {
  return task?.primarySessionState ?? "-";
}

export function didPreservePrimaryState(
  payload: TaskEventPayload,
  beforeTask: KanbanTask | undefined,
  afterTask: KanbanTask | undefined,
): boolean {
  if (payload.primary_session_state !== undefined) return false;
  const previousPrimaryState = beforeTask?.primarySessionState;
  if (previousPrimaryState === undefined) return false;
  return previousPrimaryState === afterTask?.primarySessionState;
}

function logTaskMerge(
  action: "task.created" | "task.updated" | "task.state_changed",
  beforeState: AppState,
  afterState: AppState,
  payload: TaskEventPayload,
): void {
  if (!isDebug()) return;
  const taskId = taskEventIdForLog(payload);
  const beforeTask = findTaskInState(beforeState, taskId);
  const afterTask = findTaskInState(afterState, taskId);
  lifecycleDebug(`${action} merge`, {
    task_id: taskId,
    payloadState: valueForLog(payload.state),
    payloadPrimarySessionId: valueForLog(payload.primary_session_id),
    payloadPrimarySessionState: payloadPrimaryStateForLog(payload),
    beforeTaskState: taskStateForLog(beforeTask),
    beforeTaskPrimaryState: taskPrimaryStateForLog(beforeTask),
    afterTaskState: taskStateForLog(afterTask),
    afterTaskPrimaryState: taskPrimaryStateForLog(afterTask),
    preservedPrimaryState: didPreservePrimaryState(payload, beforeTask, afterTask),
  });
}

/** Remove a task from both single-kanban and multi-kanban snapshots. */
function removeTaskFromBothKanbans(state: AppState, taskId: string): AppState {
  let next = state;
  if (state.kanban.tasks.some((t) => t.id === taskId)) {
    next = {
      ...next,
      kanban: { ...next.kanban, tasks: next.kanban.tasks.filter((t) => t.id !== taskId) },
    };
  }

  const snapshots = Object.entries(next.kanbanMulti.snapshots);
  const changedSnapshots = snapshots.filter(([, snapshot]) =>
    snapshot.tasks.some((t) => t.id === taskId),
  );
  if (changedSnapshots.length > 0) {
    const nextSnapshots = { ...next.kanbanMulti.snapshots };
    for (const [workflowId, snapshot] of changedSnapshots) {
      nextSnapshots[workflowId] = {
        ...snapshot,
        tasks: snapshot.tasks.filter((t) => t.id !== taskId),
      };
    }
    next = {
      ...next,
      kanbanMulti: {
        ...next.kanbanMulti,
        snapshots: nextSnapshots,
      },
    };
  }
  return next;
}

function clearRemovedTaskSelection(state: AppState, taskId: string): AppState {
  let next = state;
  if (next.tasks.activeTaskId === taskId) {
    next = {
      ...next,
      tasks: {
        ...next.tasks,
        activeTaskId: null,
        activeSessionId: null,
        pinnedSessionId: null,
      },
    };
  }
  if (next.tasks.lastSessionByTaskId[taskId]) {
    const { [taskId]: _, ...rest } = next.tasks.lastSessionByTaskId;
    next = { ...next, tasks: { ...next.tasks, lastSessionByTaskId: rest } };
  }
  return next;
}

function clearDeletedTaskWalkthrough(state: AppState, taskId: string): AppState {
  if (!state.walkthroughs?.byTaskId) return state;
  if (!(taskId in state.walkthroughs.byTaskId)) return state;
  const { [taskId]: _removedWalkthrough, ...byTaskId } = state.walkthroughs.byTaskId;
  const { [taskId]: _removedStep, ...activeStepByTaskId } = state.walkthroughs.activeStepByTaskId;
  const { [taskId]: _removedLastSeen, ...lastSeenUpdatedAtByTaskId } =
    state.walkthroughs.lastSeenUpdatedAtByTaskId;
  return {
    ...state,
    walkthroughs: {
      ...state.walkthroughs,
      byTaskId,
      activeStepByTaskId,
      lastSeenUpdatedAtByTaskId,
    },
  };
}

function removedTaskRedirectHref(pathname: string, taskId: string): string | null {
  if (isTaskDetailPath(pathname, taskId)) return "/";
  const normalized = normalizePathname(pathname);
  return normalized === `/office/tasks/${taskId}` ? "/office/tasks" : null;
}

/**
 * Soft-redirect away from a removed task's page. Only fires when the user is
 * currently parked on that task's route, so a background removal of some other
 * task never yanks the user elsewhere.
 */
function redirectAwayFromRemovedTask(taskId: string): void {
  if (typeof window === "undefined") return;
  const href = removedTaskRedirectHref(window.location.pathname, taskId);
  if (!href) return;
  softNavigate(href, "replace");
}

type TaskUpdatedMessage = Parameters<NonNullable<WsHandlers["task.updated"]>>[0];
type TaskCreatedMessage = Parameters<NonNullable<WsHandlers["task.created"]>>[0];
type TaskStateChangedMessage = Parameters<NonNullable<WsHandlers["task.state_changed"]>>[0];
type TaskUpsertMessage = TaskCreatedMessage | TaskStateChangedMessage;
type TaskUpsertAction = "task.created" | "task.state_changed";

function handleTaskUpdated(store: StoreApi<AppState>, message: TaskUpdatedMessage): void {
  // Skip ephemeral tasks (e.g., quick chat) - they shouldn't appear on the Kanban board
  if (message.payload.is_ephemeral) return;

  // Capture the previous primary session id BEFORE the upsert so we can
  // detect a primary-session swap (e.g. workflow profile switch reusing a
  // different session) and follow focus to the new primary.
  const beforeState = store.getState();
  const taskId = message.payload.task_id;
  const previousPrimary = findTaskInState(beforeState, taskId)?.primarySessionId ?? null;
  const archivedAt = message.payload.archived_at;

  if (archivedAt) {
    removeRecentTask(taskId);
    const state = store.getState();
    state.removeTaskFromSidebarPrefs(taskId);
    state.setOfficeRefetchTrigger("tasks");
  }

  store.setState((state) => {
    const wfId = message.payload.workflow_id;
    const oldWfId = message.payload.old_workflow_id;
    let next = state;

    if (archivedAt || (oldWfId && oldWfId !== wfId)) {
      next = removeTaskFromBothKanbans(next, taskId);
    }

    if (archivedAt) {
      return clearRemovedTaskSelection(next, taskId);
    }

    return upsertTaskInBothKanbans(next, wfId, message.payload);
  });

  if (archivedAt) {
    redirectAwayFromRemovedTask(taskId);
    return;
  }

  // Follow focus to the new primary when:
  //  - the user is currently viewing this task,
  //  - the user was sitting on the previous primary,
  //  - they do NOT have a non-terminal pinned session for this task, and
  //  - the primary actually changed.
  // This makes workflow profile switches transparent for unpinned users
  // without yanking users off a live session they deliberately selected.
  const afterState = store.getState();
  logTaskMerge("task.updated", beforeState, afterState, message.payload);
  const newPrimary = findTaskInState(afterState, taskId)?.primarySessionId ?? null;
  if (
    newPrimary &&
    newPrimary !== previousPrimary &&
    afterState.tasks.activeTaskId === taskId &&
    afterState.tasks.activeSessionId === previousPrimary &&
    !shouldPreservePinnedSessionForTask(afterState, taskId)
  ) {
    clearPinnedSessionIfOverridden(store, newPrimary);
    afterState.setActiveSessionAuto(taskId, newPrimary);
  }
}

function handleTaskUpsert(
  action: TaskUpsertAction,
  store: StoreApi<AppState>,
  message: TaskUpsertMessage,
): void {
  // Skip ephemeral tasks (e.g., quick chat) - they shouldn't appear on the Kanban board
  if (message.payload.is_ephemeral) return;

  const beforeState = store.getState();
  store.setState((state) =>
    upsertTaskInBothKanbans(state, message.payload.workflow_id, message.payload),
  );
  logTaskMerge(action, beforeState, store.getState(), message.payload);
}

export function registerTasksHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "task.created": (message) => {
      handleTaskUpsert("task.created", store, message);
    },
    "task.updated": (message) => handleTaskUpdated(store, message),
    "task.deleted": (message) => {
      const deletedId = message.payload.task_id;
      removeRecentTask(deletedId);

      const currentState = store.getState();
      const sessionIds = (currentState.taskSessionsByTask.itemsByTaskId[deletedId] ?? []).map(
        (s) => s.id,
      );
      const task = currentState.kanban.tasks.find((t) => t.id === deletedId);
      if (task?.primarySessionId) {
        const primaryId = toSessionId(task.primarySessionId);
        if (!sessionIds.includes(primaryId)) {
          sessionIds.push(primaryId);
        }
      }
      const envIds = Array.from(
        new Set(
          sessionIds
            .map((sid) => currentState.environmentIdBySessionId[sid])
            .filter((eid): eid is string => Boolean(eid)),
        ),
      );
      cleanupTaskStorage(deletedId, sessionIds, envIds);
      // Keep the in-memory sidebar pin/order arrays in sync — without this,
      // a later togglePinnedTask / setSidebarTaskOrder would persist the
      // stale state (still containing the deleted ID) back to localStorage.
      currentState.removeTaskFromSidebarPrefs(deletedId);
      for (const sid of sessionIds) {
        useContextFilesStore.getState().clearSession(sid);
      }

      const wasActive = currentState.tasks.activeTaskId === deletedId;

      store.setState((state) =>
        clearDeletedTaskWalkthrough(
          clearRemovedTaskSelection(removeTaskFromBothKanbans(state, deletedId), deletedId),
          deletedId,
        ),
      );

      // Capture the route match before any redirect mutates the pathname. This
      // covers a fresh load where the browser is parked on the task's route
      // but TaskPageContent hasn't hydrated `activeTaskId` yet, so `wasActive`
      // is still false.
      const onDeletedRoute =
        typeof window !== "undefined" &&
        removedTaskRedirectHref(window.location.pathname, deletedId) !== null;

      // Only react to genuine auto-deletions, which the backend tags with a
      // reason (e.g. a review task whose PR was approved). User-initiated deletes
      // carry no reason: their local delete flow (useTaskRemoval) owns
      // navigation by switching to the next task, so redirecting here would
      // preempt it and strand the user on the home route. For auto-deletions we
      // move off the now-dead route (helper is route-guarded) and explain why.
      if (message.payload.reason && (wasActive || onDeletedRoute)) {
        redirectAwayFromRemovedTask(deletedId);
        store.getState().setTaskDeletedNotification({
          taskId: deletedId,
          title: message.payload.title,
          reason: message.payload.reason,
        });
      }
    },
    "task.state_changed": (message) => {
      handleTaskUpsert("task.state_changed", store, message);
    },
  };
}
