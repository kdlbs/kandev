import { useCallback } from "react";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { KanbanState } from "@/lib/state/slices";
import type { TaskSession } from "@/lib/types/http";
import { linkToTaskOverview, replaceTaskUrl } from "@/lib/links";
import { fetchTask, listTaskSessions } from "@/lib/api";
import { captureTaskSessionActivityEpochs } from "@/lib/state/slices/session/activity-epochs";
import { performLayoutSwitch } from "@/lib/state/dockview-store";
import { getRecentTasks } from "@/lib/recent-tasks";
import { createAbortError, isAbortError } from "@/lib/utils/abort-error";

type TaskRemovalOptions = {
  store: StoreApi<AppState>;
  /** Whether to call performLayoutSwitch when switching sessions (desktop sidebar uses this) */
  useLayoutSwitch?: boolean;
};

export type TaskSessionLoadOptions = {
  /** Ignore the local task-session cache and request an authoritative snapshot. */
  force?: boolean;
  /** Cancel the authoritative request when its task selection is superseded. */
  signal?: AbortSignal;
};

type RemoveFromBoardOptions = {
  /**
   * The active task ID captured **before** the async delete/archive API call.
   * Only honored when the current `activeTaskId` has been cleared to `null`
   * by the WS "task.deleted" / "task.updated(archived_at)" handler racing
   * ahead of this function. If the user has manually navigated to a different
   * task during the in-flight API call, the current store value wins and
   * this captured value is ignored.
   */
  wasActiveTaskId?: string | null;
  /** The active session ID captured before the async delete API call. */
  wasActiveSessionId?: string | null;
  /** Switch away from the task without removing it from board state yet. */
  switchOnly?: boolean;
  /** Exclude the removed task and every cached descendant from candidates. */
  excludeTaskTree?: boolean;
  /** Reuse the tree captured before an archive can prune cached descendants. */
  excludedTaskIds?: ReadonlySet<string>;
};

type RemoveFromBoardResult = {
  switchedTaskId: string | null;
  excludedTaskIds?: ReadonlySet<string>;
};

const taskSessionLoadGenerations = new WeakMap<StoreApi<AppState>, Map<string, number>>();

function beginTaskSessionLoad(store: StoreApi<AppState>, taskId: string): number {
  let generations = taskSessionLoadGenerations.get(store);
  if (!generations) {
    generations = new Map();
    taskSessionLoadGenerations.set(store, generations);
  }
  const generation = (generations.get(taskId) ?? 0) + 1;
  generations.set(taskId, generation);
  return generation;
}

function taskSessionLoadIsCurrent(
  store: StoreApi<AppState>,
  taskId: string,
  generation: number,
): boolean {
  return taskSessionLoadGenerations.get(store)?.get(taskId) === generation;
}

function cachedSessionsHaveEnvIds(sessions: TaskSession[]): boolean {
  return sessions.length === 0 || sessions.every((session) => !!session.task_environment_id);
}

function taskSessionListRequestOptions(signal?: AbortSignal) {
  return signal ? { cache: "no-store" as const, init: { signal } } : { cache: "no-store" as const };
}

type TaskSessionLoadCommit = {
  sessions: TaskSession[];
  force: boolean;
  activityEpochsAtRequestStart: Readonly<Record<string, number>>;
};

function commitTaskSessionLoad(
  store: StoreApi<AppState>,
  taskId: string,
  generation: number,
  commit: TaskSessionLoadCommit,
): TaskSession[] {
  const { sessions, force, activityEpochsAtRequestStart } = commit;
  if (taskSessionLoadIsCurrent(store, taskId, generation)) {
    store.getState().setTaskSessionsForTask(taskId, sessions, activityEpochsAtRequestStart);
    return sessions;
  }
  // Forced callers use this result to choose a pending-action owner. Never
  // let a superseded response escape even though its cache write was gated.
  if (force) {
    throw createAbortError("Task session load was superseded");
  }
  return store.getState().taskSessionsByTask.itemsByTaskId[taskId] ?? [];
}

async function loadTaskSessionsForTaskFromStore(
  store: StoreApi<AppState>,
  taskId: string,
  options?: TaskSessionLoadOptions,
): Promise<TaskSession[]> {
  const state = store.getState();
  const force = options?.force === true;
  const signal = options?.signal;
  const cachedSessions = state.taskSessionsByTask.itemsByTaskId[taskId] ?? [];
  if (!force && state.taskSessionsByTask.loadedByTaskId[taskId]) {
    if (cachedSessionsHaveEnvIds(cachedSessions)) return cachedSessions;
  }
  if (!force && state.taskSessionsByTask.loadingByTaskId[taskId]) {
    return cachedSessions;
  }
  const loadGeneration = beginTaskSessionLoad(store, taskId);
  const activityEpochsAtRequestStart = captureTaskSessionActivityEpochs(store.getState(), taskId);
  store.getState().setTaskSessionsLoading(taskId, true);
  try {
    const response = await listTaskSessions(taskId, taskSessionListRequestOptions(signal));
    const sessions = response.sessions ?? [];
    return commitTaskSessionLoad(store, taskId, loadGeneration, {
      sessions,
      force,
      activityEpochsAtRequestStart,
    });
  } catch (error) {
    if (!isAbortError(error)) console.error("Failed to load task sessions:", error);
    if (force) throw error;
    return cachedSessions;
  } finally {
    if (taskSessionLoadIsCurrent(store, taskId, loadGeneration)) {
      store.getState().setTaskSessionsLoading(taskId, false);
    }
  }
}

function removeTasksFromSnapshots(store: StoreApi<AppState>, taskIds: ReadonlySet<string>): void {
  const currentSnapshots = store.getState().kanbanMulti.snapshots;
  for (const [wfId, snapshot] of Object.entries(currentSnapshots)) {
    const hadTask = snapshot.tasks.some((t: KanbanState["tasks"][number]) => taskIds.has(t.id));
    if (hadTask) {
      store.getState().setWorkflowSnapshot(wfId, {
        ...snapshot,
        tasks: snapshot.tasks.filter((t: KanbanState["tasks"][number]) => !taskIds.has(t.id)),
      });
    }
  }

  const currentKanbanTasks = store.getState().kanban.tasks;
  if (currentKanbanTasks.some((t: KanbanState["tasks"][number]) => taskIds.has(t.id))) {
    store.setState((state) => ({
      ...state,
      kanban: {
        ...state.kanban,
        tasks: state.kanban.tasks.filter((t: KanbanState["tasks"][number]) => !taskIds.has(t.id)),
      },
    }));
  }
}

function collectRemainingTasks(store: StoreApi<AppState>): KanbanState["tasks"] {
  // Keep candidate ordering snapshot-first; task-tree exclusion follows the
  // same precedence and uses kanban.tasks only to fill missing rows.
  const allRemainingTasks: KanbanState["tasks"] = [];
  for (const snapshot of Object.values(store.getState().kanbanMulti.snapshots)) {
    allRemainingTasks.push(...snapshot.tasks);
  }
  if (allRemainingTasks.length === 0) {
    allRemainingTasks.push(...store.getState().kanban.tasks);
  }
  return allRemainingTasks;
}

function collectTaskTreeIds(
  rootTaskId: string,
  taskLists: Array<KanbanState["tasks"]>,
): ReadonlySet<string> {
  const tasksById = new Map<string, KanbanState["tasks"][number]>();
  for (const tasks of taskLists) {
    for (const task of tasks) {
      if (!tasksById.has(task.id)) tasksById.set(task.id, task);
    }
  }

  const childrenByParentId = new Map<string, string[]>();
  for (const task of tasksById.values()) {
    if (!task.parentTaskId) continue;
    const children = childrenByParentId.get(task.parentTaskId) ?? [];
    children.push(task.id);
    childrenByParentId.set(task.parentTaskId, children);
  }

  const excludedTaskIds = new Set<string>([rootTaskId]);
  const pendingParentIds = [rootTaskId];
  while (pendingParentIds.length > 0) {
    const parentId = pendingParentIds.pop();
    if (!parentId) continue;
    for (const childId of childrenByParentId.get(parentId) ?? []) {
      if (excludedTaskIds.has(childId)) continue;
      excludedTaskIds.add(childId);
      pendingParentIds.push(childId);
    }
  }
  return excludedTaskIds;
}

function collectTaskTreeIdsFromStore(
  store: StoreApi<AppState>,
  rootTaskId: string,
): ReadonlySet<string> {
  const state = store.getState();
  return collectTaskTreeIds(rootTaskId, [
    ...Object.values(state.kanbanMulti.snapshots).map((snapshot) => snapshot.tasks),
    // Snapshots are the optimistic source used by the task switchers. The
    // canonical board fills gaps without overriding a duplicate snapshot row.
    state.kanban.tasks,
  ]);
}

/**
 * Orders next-task candidates by recent use, then board order, without trusting
 * either list as proof that a task still exists.
 */
function orderedTaskCandidates(
  remainingTasks: KanbanState["tasks"],
  removedTaskId: string,
  excludedTaskIds?: ReadonlySet<string>,
): KanbanState["tasks"] {
  const candidates = remainingTasks.filter(
    (task) => task.id !== removedTaskId && !excludedTaskIds?.has(task.id),
  );
  const remainingById = new Map(candidates.map((task) => [task.id, task]));
  const ordered: KanbanState["tasks"] = [];
  for (const recent of getRecentTasks()) {
    const task = remainingById.get(recent.taskId);
    if (!task) continue;
    ordered.push(task);
    remainingById.delete(task.id);
  }
  ordered.push(...candidates.filter((task) => remainingById.has(task.id)));
  return ordered;
}

async function taskIsLive(taskId: string): Promise<boolean> {
  try {
    const task = await fetchTask(taskId, { cache: "no-store" });
    return !task.archived_at;
  } catch {
    return false;
  }
}

/**
 * Picks the first candidate that the task API still reports as unarchived.
 * Workflow snapshots and the active kanban are both cached projections and can
 * lag the same delete/archive event, so neither can independently validate
 * membership.
 */
export async function selectNextTaskAfterRemoval(
  remainingTasks: KanbanState["tasks"],
  removedTaskId: string,
  isLive: (taskId: string) => Promise<boolean> = taskIsLive,
  excludedTaskIds?: ReadonlySet<string>,
): Promise<KanbanState["tasks"][number] | null> {
  for (const task of orderedTaskCandidates(remainingTasks, removedTaskId, excludedTaskIds)) {
    if (await isLive(task.id)) return task;
  }
  return null;
}

function switchToSessionForTask(params: {
  store: StoreApi<AppState>;
  nextTask: KanbanState["tasks"][number];
  sessionId: string;
  oldEnvId: string | null;
  useLayoutSwitch: boolean;
}): void {
  const { store, nextTask, sessionId, oldEnvId, useLayoutSwitch } = params;
  store.getState().setActiveSession(nextTask.id, sessionId);
  if (!useLayoutSwitch) return;
  const state = store.getState();
  const newEnvId = state.environmentIdBySessionId[sessionId] ?? null;
  const sessionIds = (state.taskSessionsByTask.itemsByTaskId[nextTask.id] ?? []).map(
    (session) => session.id,
  );
  if (newEnvId) performLayoutSwitch(oldEnvId, newEnvId, sessionId, sessionIds);
}

async function switchToNextTask(params: {
  store: StoreApi<AppState>;
  nextTask: KanbanState["tasks"][number];
  oldEnvId: string | null;
  useLayoutSwitch: boolean;
  loadTaskSessionsForTask: (taskId: string) => Promise<TaskSession[]>;
}): Promise<void> {
  const { store, nextTask, oldEnvId, useLayoutSwitch, loadTaskSessionsForTask } = params;
  if (nextTask.primarySessionId) {
    if (useLayoutSwitch && !store.getState().environmentIdBySessionId[nextTask.primarySessionId]) {
      await loadTaskSessionsForTask(nextTask.id);
    }
    switchToSessionForTask({
      store,
      nextTask,
      sessionId: nextTask.primarySessionId,
      oldEnvId,
      useLayoutSwitch,
    });
    replaceTaskUrl(nextTask.id);
    return;
  }

  const sessions = await loadTaskSessionsForTask(nextTask.id);
  const sessionId = sessions[0]?.id ?? null;
  if (sessionId) {
    switchToSessionForTask({ store, nextTask, sessionId, oldEnvId, useLayoutSwitch });
  } else {
    store.getState().setActiveTask(nextTask.id);
  }
  replaceTaskUrl(nextTask.id);
}

function resolveOldEnvId(store: StoreApi<AppState>, opts?: RemoveFromBoardOptions): string | null {
  const oldSessionId =
    opts?.wasActiveSessionId !== undefined
      ? opts.wasActiveSessionId
      : store.getState().tasks.activeSessionId;
  return oldSessionId ? (store.getState().environmentIdBySessionId[oldSessionId] ?? null) : null;
}

/**
 * Decide whether the removed task is the one the user is currently viewing.
 *
 * Two cases count as "still on the removed task":
 *   1. `stillOnRemoved` — the store's current `activeTaskId` matches `taskId`.
 *   2. `wsCleared` — the store's `activeTaskId` has been cleared to `null`
 *      (the WS `task.deleted` / `task.updated(archived_at)` handler raced
 *      ahead of us) AND the caller-captured `wasActiveTaskId` matches `taskId`.
 *
 * Any other state means the user manually moved to a different task during
 * the in-flight API call — leave them on their chosen task.
 */
function shouldSwitchAfterRemoval(
  store: StoreApi<AppState>,
  taskId: string,
  opts?: RemoveFromBoardOptions,
): boolean {
  const currentActiveTaskId = store.getState().tasks.activeTaskId;
  const stillOnRemoved = currentActiveTaskId === taskId;
  const wsCleared = currentActiveTaskId === null && opts?.wasActiveTaskId === taskId;
  return stillOnRemoved || wsCleared;
}

/**
 * Hook that provides shared logic for removing a task from the kanban board
 * (after archive or delete) and switching to the next available task.
 *
 * Used by both TaskSessionSidebar and SessionTaskSwitcherSheet.
 */
export function useTaskRemoval({ store, useLayoutSwitch = false }: TaskRemovalOptions) {
  const loadTaskSessionsForTask = useCallback(
    (taskId: string, options?: TaskSessionLoadOptions) =>
      loadTaskSessionsForTaskFromStore(store, taskId, options),
    [store],
  );

  /**
   * Remove a task from the kanban board state (both single and multi snapshots)
   * and switch to the next available task if the removed task was active.
   *
   * Pass `opts.wasActiveTaskId` / `opts.wasActiveSessionId` when calling after
   * an async API call (e.g. deleteTaskById, archiveTask) — the WS handler may
   * clear activeTaskId before this function runs. The captured value is only
   * consulted as a fallback when the current store value has been cleared; if
   * the user manually navigated to a different task mid-flight, the store
   * wins and the captured value is ignored (no auto-switch).
   */
  const removeTaskFromBoard = useCallback(
    async (taskId: string, opts?: RemoveFromBoardOptions): Promise<RemoveFromBoardResult> => {
      const excludedTaskIds = opts?.excludeTaskTree
        ? (opts.excludedTaskIds ?? collectTaskTreeIdsFromStore(store, taskId))
        : undefined;
      if (!opts?.switchOnly) {
        // A cascade archive publishes one task.updated event per descendant,
        // but those events can arrive after the archive request resolves. The
        // cached tree is already known to be removed at this point, so prune
        // it optimistically and let WS updates reconcile any other clients.
        removeTasksFromSnapshots(store, excludedTaskIds ?? new Set([taskId]));
      }
      const allRemainingTasks = collectRemainingTasks(store);

      if (!shouldSwitchAfterRemoval(store, taskId, opts)) {
        return { switchedTaskId: null, excludedTaskIds };
      }

      const oldEnvId = resolveOldEnvId(store, opts);
      const nextTask = await selectNextTaskAfterRemoval(
        allRemainingTasks,
        taskId,
        taskIsLive,
        excludedTaskIds,
      );
      if (nextTask) {
        await switchToNextTask({
          store,
          nextTask,
          oldEnvId,
          useLayoutSwitch,
          loadTaskSessionsForTask,
        });
        return { switchedTaskId: nextTask.id, excludedTaskIds };
      }

      // When switchOnly=true and no safe candidate exists, defer Home until
      // the post-archive cleanup confirms that the request succeeded.
      if (opts?.switchOnly) return { switchedTaskId: null, excludedTaskIds };
      window.location.href = linkToTaskOverview();
      return { switchedTaskId: null, excludedTaskIds };
    },
    [store, useLayoutSwitch, loadTaskSessionsForTask],
  );

  return { removeTaskFromBoard, loadTaskSessionsForTask };
}
