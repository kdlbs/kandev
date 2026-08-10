import type { StoreApi } from "zustand";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import { cleanupTaskStorage } from "@/lib/local-storage";
import { removeRecentTask } from "@/lib/recent-tasks";
import { useContextFilesStore } from "@/lib/state/context-files-store";
import { toKanbanTask } from "@/lib/kanban/map-task";
import { sessionId as toSessionId } from "@/lib/types/http";
import { mergeTaskRepositoryFields } from "@/lib/ws/handlers/task-repositories";
import { softNavigate } from "@/lib/routing/client-router";
import { isTaskDetailPath, linkToTaskOverview, normalizePathname } from "@/lib/links";
import {
  clearPinnedSessionIfOverridden,
  shouldPreservePinnedSessionForTask,
} from "@/lib/ws/handlers/agent-session";
import { syncQuickChatFromTaskEvent } from "@/lib/ws/handlers/quick-chat";
import {
  archivedTaskWorkspaceId,
  findArchivedTaskInCache,
  removeTaskFromActiveKanbans,
  removeTaskFromBothKanbans,
  type KanbanTask,
  type TaskEventPayload,
} from "@/lib/ws/handlers/task-archive-cache";
const lifecycleDebug = createDebugLogger("task-lifecycle:ws");

function hasPayloadField(payload: TaskEventPayload, field: keyof TaskEventPayload): boolean {
  return Object.prototype.hasOwnProperty.call(payload, field);
}

function preservePrimaryExecutorFields(
  existing: KanbanTask,
  merged: KanbanTask,
  payload: TaskEventPayload,
): void {
  if (!hasPayloadField(payload, "autopilot")) merged.autopilot = existing.autopilot;
  const primarySessionCleared =
    hasPayloadField(payload, "primary_session_id") && payload.primary_session_id === null;
  if (primarySessionCleared) return;
  if (!hasPayloadField(payload, "primary_executor_id")) {
    merged.primaryExecutorId = existing.primaryExecutorId;
  }
  if (!hasPayloadField(payload, "primary_executor_type")) {
    merged.primaryExecutorType = existing.primaryExecutorType;
  }
  if (!hasPayloadField(payload, "primary_executor_name")) {
    merged.primaryExecutorName = existing.primaryExecutorName;
  }
  if (!hasPayloadField(payload, "is_remote_executor")) {
    merged.isRemoteExecutor = existing.isRemoteExecutor;
  }
}

// A lightweight task.updated may omit an unchanged field; only an explicit
// value (true/false, null, an object) may change the cached reading. This is
// the shared preserve guard for every field that follows that contract —
// interrupted, status_summary, and parent_id (an explicit `parent_id: null`
// means detach; see parentIDEventField in service_tasks.go).
function preserveOmittedField<K extends keyof KanbanTask>(
  existing: KanbanTask | undefined,
  merged: KanbanTask,
  payload: TaskEventPayload,
  nextTask: KanbanTask,
  field: { payloadKey: keyof TaskEventPayload; taskField: K },
): void {
  if (!hasPayloadField(payload, field.payloadKey) && nextTask[field.taskField] === undefined) {
    // The three call sites pass optional fields (interrupted, statusSummary,
    // parentTaskId), so the undefined value is the safe "not present" reading.
    merged[field.taskField] = existing?.[field.taskField] as KanbanTask[K];
  }
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
  preserveOmittedField(existing, merged, payload, nextTask, {
    payloadKey: "parent_id",
    taskField: "parentTaskId",
  });
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
  preservePrimaryExecutorFields(existing, merged, payload);
  if (!hasPayloadField(payload, "metadata")) merged.metadata = existing.metadata;
  if (
    !hasPayloadField(payload, "task_pending_action") &&
    nextTask.taskPendingAction === undefined
  ) {
    merged.taskPendingAction = existing.taskPendingAction;
  }
  // Preserve the task-level activity aggregate only when the event omits it
  // entirely (e.g. a lightweight kanban.update). A task.updated that carries an
  // explicit null clears a stale background-running reading, so it must win.
  if (
    !hasPayloadField(payload, "foreground_activity") &&
    nextTask.foregroundActivity === undefined
  ) {
    merged.foregroundActivity = existing.foregroundActivity;
  }
  preserveOmittedField(existing, merged, payload, nextTask, {
    payloadKey: "interrupted",
    taskField: "interrupted",
  });
  if (
    !hasPayloadField(payload, "active_subagent_count") &&
    nextTask.activeSubagentCount === undefined
  ) {
    merged.activeSubagentCount = existing.activeSubagentCount;
  }
  preserveOmittedField(existing, merged, payload, nextTask, {
    payloadKey: "status_summary",
    taskField: "statusSummary",
  });
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

function upsertArchivedTaskInCache(
  state: AppState,
  payload: TaskEventPayload,
  resolvedWorkspaceId?: string,
): AppState {
  const workspaceId = resolvedWorkspaceId ?? archivedTaskWorkspaceId(state, payload);
  if (!workspaceId || payload.is_ephemeral) return state;
  const task = { ...toKanbanTask(payload), workspaceId, isArchived: true };
  const sidebarArchivedTasks = state.sidebarArchivedTasks ?? {
    itemsByWorkspaceId: {},
    loadedByWorkspaceId: {},
    loadingByWorkspaceId: {},
    errorByWorkspaceId: {},
    revisionByWorkspaceId: {},
  };
  const items = sidebarArchivedTasks.itemsByWorkspaceId[workspaceId] ?? [];
  const existing = items.find((item) => item.id === task.id);
  const merged = mergeTaskUpdate(existing, task, payload);
  const revisions = sidebarArchivedTasks.revisionByWorkspaceId ?? {};
  return {
    ...state,
    sidebarArchivedTasks: {
      ...sidebarArchivedTasks,
      itemsByWorkspaceId: {
        ...sidebarArchivedTasks.itemsByWorkspaceId,
        [workspaceId]: existing
          ? items.map((item) => (item.id === task.id ? merged : item))
          : [...items, merged],
      },
      revisionByWorkspaceId: {
        ...revisions,
        [workspaceId]: (revisions[workspaceId] ?? 0) + 1,
      },
    },
  };
}

function removeArchivedTaskFromCache(state: AppState, taskId: string): AppState {
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
  if (isTaskDetailPath(pathname, taskId)) return linkToTaskOverview();
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
type TaskStatusSummaryUpdatedMessage = Parameters<
  NonNullable<WsHandlers["task.status_summary.updated"]>
>[0];
type TaskUpsertMessage = TaskCreatedMessage | TaskStateChangedMessage;
type TaskUpsertAction = "task.created" | "task.state_changed";

function updateTaskStatusSummaryInBothKanbans(
  state: AppState,
  message: TaskStatusSummaryUpdatedMessage,
): AppState {
  const { task_id: taskId, status_summary: nextSummary } = message.payload;
  const shouldReplace = (task: KanbanTask): boolean => {
    const current = task.statusSummary;
    return !current || nextSummary.revision > current.revision;
  };
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
  if (changedSnapshots.length === 0) return next;

  const nextSnapshots = { ...next.kanbanMulti.snapshots };
  for (const [workflowId, snapshot] of changedSnapshots) {
    nextSnapshots[workflowId] = {
      ...snapshot,
      tasks: snapshot.tasks.map((task) => (task.id === taskId ? updateTask(task) : task)),
    };
  }
  return {
    ...next,
    kanbanMulti: { ...next.kanbanMulti, snapshots: nextSnapshots },
  };
}

type TaskUpdatedCacheContext = {
  state: AppState;
  taskId: string;
  workflowId: string;
  oldWorkflowId?: string | null;
  payload: TaskEventPayload;
  isArchivedUpdate: boolean;
  partialArchivedTask?: KanbanTask;
  archivedAt?: string | null;
  archivedWorkspaceId?: string;
};

function applyTaskUpdatedCache({
  state,
  taskId,
  workflowId,
  oldWorkflowId,
  payload,
  isArchivedUpdate,
  partialArchivedTask,
  archivedAt,
  archivedWorkspaceId,
}: TaskUpdatedCacheContext): AppState {
  let next = state;

  if (isArchivedUpdate) {
    next =
      partialArchivedTask && !archivedAt
        ? removeTaskFromActiveKanbans(next, taskId)
        : removeTaskFromBothKanbans(next, taskId);
  }

  if (isArchivedUpdate) {
    const archivedState = upsertArchivedTaskInCache(next, payload, archivedWorkspaceId);
    return archivedAt ? clearRemovedTaskSelection(archivedState, taskId) : archivedState;
  }

  if (oldWorkflowId && oldWorkflowId !== workflowId) {
    next = removeTaskFromBothKanbans(next, taskId);
  }

  return upsertTaskInBothKanbans(removeArchivedTaskFromCache(next, taskId), workflowId, payload);
}

type TaskUpdatedArchiveContext = {
  archivedAt?: string | null;
  partialArchivedTask?: KanbanTask;
  isArchivedUpdate: boolean;
  archivedWorkspaceId?: string;
};

function getTaskUpdatedArchiveContext(
  state: AppState,
  payload: TaskEventPayload,
  taskId: string,
): TaskUpdatedArchiveContext {
  const hasArchivedAt = hasPayloadField(payload, "archived_at");
  const archivedAt = payload.archived_at;
  const partialArchivedTask = !hasArchivedAt ? findArchivedTaskInCache(state, taskId) : undefined;
  const isArchivedUpdate = Boolean(archivedAt || partialArchivedTask);
  const archivedWorkspaceId = archivedAt
    ? archivedTaskWorkspaceId(state, payload)
    : partialArchivedTask?.workspaceId;
  return { archivedAt, partialArchivedTask, isArchivedUpdate, archivedWorkspaceId };
}

function removeArchivedTaskSideEffects(store: StoreApi<AppState>, taskId: string): void {
  removeRecentTask(taskId);
  const state = store.getState();
  state.removeTaskFromSidebarPrefs(taskId);
  state.setOfficeRefetchTrigger("tasks");
}

function maybeFollowUpdatedTaskPrimary(
  store: StoreApi<AppState>,
  beforeState: AppState,
  taskId: string,
  previousPrimary: string | null,
  payload: TaskEventPayload,
): void {
  // Follow focus to the new primary when:
  //  - the user is currently viewing this task,
  //  - the user was sitting on the previous primary,
  //  - they do NOT have a non-terminal pinned session for this task, and
  //  - a real previous primary actually changed.
  // This makes workflow profile switches transparent for unpinned users
  // without yanking users off a live session they deliberately selected. A
  // pinned user whose session is being retired is followed via the session
  // state-transition handoff (maybeAdoptSessionOnTransition) once that session
  // actually reaches a terminal state — not from here, where we cannot yet tell
  // a retirement from a manual "Set as Primary" that leaves the old session live.
  const afterState = store.getState();
  logTaskMerge("task.updated", beforeState, afterState, payload);
  const newPrimary = findTaskInState(afterState, taskId)?.primarySessionId ?? null;
  const shouldFollow =
    Boolean(newPrimary) &&
    Boolean(previousPrimary) &&
    newPrimary !== previousPrimary &&
    afterState.tasks.activeTaskId === taskId &&
    afterState.tasks.activeSessionId === previousPrimary &&
    !shouldPreservePinnedSessionForTask(afterState, taskId);
  if (!shouldFollow || !newPrimary) return;
  clearPinnedSessionIfOverridden(store, newPrimary);
  afterState.setActiveSessionAuto(taskId, newPrimary);
}

function handleTaskUpdated(store: StoreApi<AppState>, message: TaskUpdatedMessage): void {
  // Ephemeral tasks never reach the Kanban board, but quick chats are shared
  // across devices — mirror them into the tab strip before bailing out.
  if (message.payload.is_ephemeral) {
    syncQuickChatFromTaskEvent(store, message.payload);
    return;
  }

  // Capture the previous primary session id BEFORE the upsert so we can
  // detect a primary-session swap (e.g. workflow profile switch reusing a
  // different session) and follow focus to the new primary.
  const beforeState = store.getState();
  const taskId = message.payload.task_id;
  const previousPrimary = findTaskInState(beforeState, taskId)?.primarySessionId ?? null;
  const { archivedAt, partialArchivedTask, isArchivedUpdate, archivedWorkspaceId } =
    getTaskUpdatedArchiveContext(beforeState, message.payload, taskId);

  if (archivedAt) {
    removeArchivedTaskSideEffects(store, taskId);
  }

  store.setState((state) =>
    applyTaskUpdatedCache({
      state,
      taskId,
      workflowId: message.payload.workflow_id,
      oldWorkflowId: message.payload.old_workflow_id,
      payload: message.payload,
      isArchivedUpdate,
      partialArchivedTask,
      archivedAt,
      archivedWorkspaceId,
    }),
  );

  if (archivedAt) {
    redirectAwayFromRemovedTask(taskId);
    return;
  }

  maybeFollowUpdatedTaskPrimary(store, beforeState, taskId, previousPrimary, message.payload);
}

function handleTaskUpsert(
  action: TaskUpsertAction,
  store: StoreApi<AppState>,
  message: TaskUpsertMessage,
): void {
  // See handleTaskUpdated: off the board, but still a quick-chat tab.
  if (message.payload.is_ephemeral) {
    syncQuickChatFromTaskEvent(store, message.payload);
    return;
  }

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
      // A quick chat closed on another device must not linger here as a tab
      // pointing at a task the backend already deleted.
      store.getState().removeQuickChatSessionsForTask(deletedId);

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
      // Remove the deleted task before the next sidebar preference PATCH.
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
    "task.status_summary.updated": (message) => {
      store.setState((state) => updateTaskStatusSummaryInBothKanbans(state, message));
    },
  };
}
