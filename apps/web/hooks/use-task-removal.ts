import { useCallback } from "react";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { KanbanState } from "@/lib/state/slices";
import type { Task, TaskSession } from "@/lib/types/http";
import { linkToTask, linkToTaskOverview } from "@/lib/links";
import { fetchTask, listTaskSessions } from "@/lib/api";
import { captureTaskSessionActivityEpochs } from "@/lib/state/slices/session/activity-epochs";
import { performLayoutSwitch } from "@/lib/state/dockview-store";
import { getRecentTasks } from "@/lib/recent-tasks";
import { createAbortError, isAbortError } from "@/lib/utils/abort-error";
import { softNavigate } from "@/lib/routing/client-router";
import { ownsTaskRemovalDeparture, type TaskRemovalAction } from "@/lib/state/task-removal";
import { coordinateTaskRemovalBatch } from "./task-removal-coordinator";
import { useToast } from "@/components/toast-provider";
import { useTranslation } from "react-i18next";

type TaskRemovalOptions = {
  store: StoreApi<AppState>;
  /** Whether to call performLayoutSwitch when switching sessions (desktop sidebar uses this) */
  useLayoutSwitch?: boolean;
  /** Show one localized success message after a fully successful operation. */
  notifySuccess?: TaskRemovalSuccessNotifier;
};

export type TaskRemovalSuccessNotifier = (action: TaskRemovalAction, count: number) => void;

export type TaskSessionLoadOptions = {
  /** Ignore the local task-session cache and request an authoritative snapshot. */
  force?: boolean;
  /** Cancel the authoritative request when its task selection is superseded. */
  signal?: AbortSignal;
};

export type RemoveFromBoardOptions = {
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
  /** Remove this exact set from cached board projections after success. */
  removedTaskIds?: ReadonlySet<string>;
  /** Guard automatic selection and URL writes for a coordinated operation. */
  removalToken?: string;
  /** Navigation revision captured when the operation was accepted. */
  removalNavigationRevision?: number;
  /** Restrict fallback candidates to the current workspace projection. */
  workspaceId?: string | null;
  /** Revalidate parent ancestry when the removal set came from a cascade. */
  validateTaskAncestry?: boolean;
};

export type RemoveFromBoardResult = {
  switchedTaskId: string | null;
  excludedTaskIds?: ReadonlySet<string>;
};

export type TaskRemovalRunOptions = {
  cascade?: boolean;
  workspaceId?: string | null;
};

export type TaskRemovalRequest = {
  taskId: string;
  mutate: () => Promise<void>;
};

export type TaskRemovalRunResult = {
  skipped: boolean;
  operationToken: string | null;
  switchedTaskId: string | null;
  succeededTaskIds: string[];
  failedTaskIds: string[];
};

export type TaskRemovalBatchResult = TaskRemovalRunResult & {
  errorsByTaskId: Record<string, unknown>;
};

export function useTaskRemovalSuccessNotifier(): TaskRemovalSuccessNotifier {
  const { toast } = useToast();
  const { t } = useTranslation();
  return useCallback(
    (action: TaskRemovalAction, count: number) => {
      const key = action === "archive" ? "tasks:taskArchiveCompleted" : "tasks:taskDeleteCompleted";
      toast({ title: t(key, { count }), variant: "success" });
    },
    [t, toast],
  );
}

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
  // Keep candidate ordering snapshot-first; the canonical board fills missing
  // rows without duplicating a task present in a workflow snapshot.
  const allRemainingTasks: KanbanState["tasks"] = [];
  const seen = new Set<string>();
  for (const snapshot of Object.values(store.getState().kanbanMulti.snapshots)) {
    for (const task of snapshot.tasks) {
      if (seen.has(task.id)) continue;
      seen.add(task.id);
      allRemainingTasks.push(task);
    }
  }
  for (const task of store.getState().kanban.tasks) {
    if (seen.has(task.id)) continue;
    seen.add(task.id);
    allRemainingTasks.push(task);
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
  workspaceId?: string | null,
): KanbanState["tasks"] {
  const candidates = remainingTasks.filter(
    (task) =>
      task.id !== removedTaskId &&
      !excludedTaskIds?.has(task.id) &&
      (!workspaceId || task.workspaceId === workspaceId),
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

async function taskHasSafeRemovalAncestry(
  taskId: string,
  excludedTaskIds: ReadonlySet<string>,
  workspaceId?: string | null,
): Promise<boolean> {
  const visited = new Set<string>();
  let currentTaskId: string | null = taskId;

  while (currentTaskId) {
    if (visited.has(currentTaskId)) return false;
    visited.add(currentTaskId);

    let task: Task;
    try {
      task = await fetchTask(currentTaskId, { cache: "no-store" });
    } catch {
      return false;
    }
    if (task.archived_at || (workspaceId && task.workspace_id !== workspaceId)) return false;

    const parentId = task.parent_id ?? null;
    if (!parentId) return true;
    if (excludedTaskIds.has(parentId)) return false;
    currentTaskId = parentId;
  }

  return false;
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
  options?: {
    excludedTaskIds?: ReadonlySet<string>;
    workspaceId?: string | null;
    validateTaskAncestry?: boolean;
  },
): Promise<KanbanState["tasks"][number] | null> {
  const { excludedTaskIds, workspaceId, validateTaskAncestry = false } = options ?? {};
  for (const task of orderedTaskCandidates(
    remainingTasks,
    removedTaskId,
    excludedTaskIds,
    workspaceId,
  )) {
    if (
      validateTaskAncestry &&
      !(await taskHasSafeRemovalAncestry(task.id, excludedTaskIds ?? new Set(), workspaceId))
    ) {
      continue;
    }
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
  setActiveSessionAutomatically(store, nextTask.id, sessionId);
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
  canCommit?: () => boolean;
}): Promise<boolean> {
  const { store, nextTask, oldEnvId, useLayoutSwitch, loadTaskSessionsForTask, canCommit } = params;
  if (nextTask.primarySessionId) {
    if (useLayoutSwitch && !store.getState().environmentIdBySessionId[nextTask.primarySessionId]) {
      await loadTaskSessionsForTask(nextTask.id);
    }
    if (canCommit && !canCommit()) return false;
    switchToSessionForTask({
      store,
      nextTask,
      sessionId: nextTask.primarySessionId,
      oldEnvId,
      useLayoutSwitch,
    });
    softNavigate(linkToTask(nextTask.id), "replace");
    return true;
  }

  const sessions = await loadTaskSessionsForTask(nextTask.id);
  if (canCommit && !canCommit()) return false;
  const sessionId = sessions[0]?.id ?? null;
  if (sessionId) {
    switchToSessionForTask({ store, nextTask, sessionId, oldEnvId, useLayoutSwitch });
  } else {
    setActiveTaskAutomatically(store, nextTask.id);
  }
  softNavigate(linkToTask(nextTask.id), "replace");
  return true;
}

function resolveOldEnvId(store: StoreApi<AppState>, opts?: RemoveFromBoardOptions): string | null {
  const oldSessionId =
    opts?.wasActiveSessionId !== undefined
      ? opts.wasActiveSessionId
      : store.getState().tasks.activeSessionId;
  return oldSessionId ? (store.getState().environmentIdBySessionId[oldSessionId] ?? null) : null;
}

function taskTreeIdsForRequests(
  store: StoreApi<AppState>,
  taskIds: string[],
  cascade: boolean,
): Map<string, ReadonlySet<string>> {
  return new Map(
    taskIds.map((taskId) => [
      taskId,
      cascade ? collectTaskTreeIdsFromStore(store, taskId) : new Set([taskId]),
    ]),
  );
}

function setActiveTaskAutomatically(store: StoreApi<AppState>, taskId: string): void {
  const state = store.getState() as AppState & {
    setActiveTaskAuto?: (id: string) => void;
  };
  if (state.setActiveTaskAuto) state.setActiveTaskAuto(taskId);
  else state.setActiveTask(taskId);
}

function setActiveSessionAutomatically(
  store: StoreApi<AppState>,
  taskId: string,
  sessionId: string,
): void {
  const state = store.getState() as AppState & {
    setActiveSessionAuto?: (task: string, session: string) => void;
  };
  if (state.setActiveSessionAuto) state.setActiveSessionAuto(taskId, sessionId);
  else state.setActiveSession(taskId, sessionId);
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
  const stillOnRemoved =
    currentActiveTaskId === taskId ||
    Boolean(currentActiveTaskId && opts?.excludedTaskIds?.has(currentActiveTaskId));
  const wsCleared =
    currentActiveTaskId === null &&
    Boolean(
      opts?.wasActiveTaskId &&
      (opts.wasActiveTaskId === taskId || opts.excludedTaskIds?.has(opts.wasActiveTaskId)),
    );
  return stillOnRemoved || wsCleared;
}

function removalTaskIdsForBoard(
  store: StoreApi<AppState>,
  taskId: string,
  opts?: RemoveFromBoardOptions,
): { excludedTaskIds?: ReadonlySet<string>; removedTaskIds: ReadonlySet<string> } {
  const excludedTaskIds =
    opts?.excludedTaskIds ??
    (opts?.excludeTaskTree ? collectTaskTreeIdsFromStore(store, taskId) : undefined);
  return {
    excludedTaskIds,
    removedTaskIds: opts?.removedTaskIds ?? excludedTaskIds ?? new Set([taskId]),
  };
}

async function switchAfterRemoval(params: {
  store: StoreApi<AppState>;
  taskId: string;
  opts?: RemoveFromBoardOptions;
  excludedTaskIds?: ReadonlySet<string>;
  oldEnvId: string | null;
  useLayoutSwitch: boolean;
  loadTaskSessionsForTask: (taskId: string) => Promise<TaskSession[]>;
}): Promise<{ candidateFound: boolean; switchedTaskId: string | null }> {
  const {
    store,
    taskId,
    opts,
    excludedTaskIds,
    oldEnvId,
    useLayoutSwitch,
    loadTaskSessionsForTask,
  } = params;
  const nextTask = await selectNextTaskAfterRemoval(
    collectRemainingTasks(store),
    taskId,
    taskIsLive,
    {
      excludedTaskIds,
      workspaceId: opts?.workspaceId,
      validateTaskAncestry: opts?.validateTaskAncestry,
    },
  );
  if (!nextTask) return { candidateFound: false, switchedTaskId: null };

  const switched = await switchToNextTask({
    store,
    nextTask,
    oldEnvId,
    useLayoutSwitch,
    loadTaskSessionsForTask,
    canCommit:
      opts?.removalToken && opts.removalNavigationRevision !== undefined
        ? () =>
            ownsTaskRemovalDeparture(
              store.getState().taskRemoval,
              opts.removalToken!,
              opts.removalNavigationRevision!,
            )
        : undefined,
  });
  return { candidateFound: true, switchedTaskId: switched ? nextTask.id : null };
}

function shouldRedirectAfterNoRemovalCandidate(
  store: StoreApi<AppState>,
  opts: RemoveFromBoardOptions | undefined,
): boolean {
  if (opts?.switchOnly) return false;
  if (
    opts?.removalToken &&
    opts.removalNavigationRevision !== undefined &&
    !ownsTaskRemovalDeparture(
      store.getState().taskRemoval,
      opts.removalToken,
      opts.removalNavigationRevision,
    )
  ) {
    return false;
  }
  return true;
}

async function removeTaskFromBoardFromStore(params: {
  store: StoreApi<AppState>;
  taskId: string;
  opts?: RemoveFromBoardOptions;
  useLayoutSwitch: boolean;
  loadTaskSessionsForTask: (taskId: string) => Promise<TaskSession[]>;
}): Promise<RemoveFromBoardResult> {
  const { store, taskId, opts, useLayoutSwitch, loadTaskSessionsForTask } = params;
  const { excludedTaskIds, removedTaskIds } = removalTaskIdsForBoard(store, taskId, opts);
  if (!opts?.switchOnly) removeTasksFromSnapshots(store, removedTaskIds);
  if (!shouldSwitchAfterRemoval(store, taskId, opts)) {
    return { switchedTaskId: null, excludedTaskIds };
  }

  const switchResult = await switchAfterRemoval({
    store,
    taskId,
    opts,
    excludedTaskIds,
    oldEnvId: resolveOldEnvId(store, opts),
    useLayoutSwitch,
    loadTaskSessionsForTask,
  });
  if (switchResult.candidateFound) return switchResult;
  if (shouldRedirectAfterNoRemovalCandidate(store, opts)) {
    softNavigate(linkToTaskOverview(), "replace");
  }
  return { switchedTaskId: null, excludedTaskIds };
}

/**
 * Hook that provides shared logic for removing a task from the kanban board
 * (after archive or delete) and switching to the next available task.
 *
 * Used by both TaskSessionSidebar and SessionTaskSwitcherSheet.
 */
export function useTaskRemoval({
  store,
  useLayoutSwitch = false,
  notifySuccess,
}: TaskRemovalOptions) {
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
    (taskId: string, opts?: RemoveFromBoardOptions) =>
      removeTaskFromBoardFromStore({
        store,
        taskId,
        opts,
        useLayoutSwitch,
        loadTaskSessionsForTask,
      }),
    [store, useLayoutSwitch, loadTaskSessionsForTask],
  );

  const getRemovalIds = useCallback(
    (requestIds: string[], cascade: boolean) => taskTreeIdsForRequests(store, requestIds, cascade),
    [store],
  );

  const runTaskRemovalBatch = useCallback(
    (
      action: TaskRemovalAction,
      requests: TaskRemovalRequest[],
      opts?: TaskRemovalRunOptions,
    ): Promise<TaskRemovalBatchResult> =>
      coordinateTaskRemovalBatch(
        { store, removeTaskFromBoard, getRemovalIds, notifySuccess },
        action,
        requests,
        opts,
      ),
    [getRemovalIds, notifySuccess, removeTaskFromBoard, store],
  );

  const runTaskRemoval = useCallback(
    async (
      action: TaskRemovalAction,
      request: TaskRemovalRequest,
      opts?: TaskRemovalRunOptions,
    ): Promise<TaskRemovalRunResult> => {
      const result = await runTaskRemovalBatch(action, [request], opts);
      if (result.failedTaskIds.length > 0) {
        throw result.errorsByTaskId[request.taskId] ?? new Error();
      }
      return result;
    },
    [runTaskRemovalBatch],
  );

  return {
    removeTaskFromBoard,
    loadTaskSessionsForTask,
    runTaskRemoval,
    runTaskRemovalBatch,
  };
}
