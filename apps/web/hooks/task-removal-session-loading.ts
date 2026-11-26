import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";
import { listTaskSessions } from "@/lib/api";
import { captureTaskSessionHydrationEpochs } from "@/lib/state/slices/session/hydration-epochs";
import { getTaskSessionReads, type TaskSessionReads } from "@/lib/state/task-session-reads";
import { createAbortError, isAbortError } from "@/lib/utils/abort-error";

export type TaskSessionLoadOptions = {
  /** Ignore the local task-session cache and request an authoritative snapshot. */
  force?: boolean;
  /** Cancel the authoritative request when its task selection is superseded. */
  signal?: AbortSignal;
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

function settleTaskSessionLoad(
  store: StoreApi<AppState>,
  taskId: string,
  generation: number,
  reads: TaskSessionReads,
): void {
  if (
    taskSessionLoadIsCurrent(store, taskId, generation) &&
    reads.isCurrent() &&
    !reads.isReading(taskId)
  ) {
    store.getState().setTaskSessionsLoading(taskId, false);
  }
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
  hydrationEpochsAtRequestStart: ReturnType<typeof captureTaskSessionHydrationEpochs>;
  membershipIsCurrent: () => boolean;
};

function commitTaskSessionLoad(
  store: StoreApi<AppState>,
  taskId: string,
  generation: number,
  commit: TaskSessionLoadCommit,
): TaskSession[] {
  const { sessions, force, hydrationEpochsAtRequestStart } = commit;
  if (taskSessionLoadIsCurrent(store, taskId, generation) && commit.membershipIsCurrent()) {
    store.getState().setTaskSessionsForTask(taskId, sessions, hydrationEpochsAtRequestStart);
    return sessions;
  }
  // Forced callers use this result to choose a pending-action owner. Never
  // let a superseded response escape even though its cache write was gated.
  if (force) {
    // i18n-exempt: Internal cancellation reason; callers classify AbortError.
    throw createAbortError("Task session load was superseded");
  }
  return store.getState().taskSessionsByTask.itemsByTaskId[taskId] ?? [];
}

export async function loadTaskSessionsForTaskFromStore(
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
  const hydrationEpochsAtRequestStart = captureTaskSessionHydrationEpochs(store.getState(), taskId);
  const reads = getTaskSessionReads(store);
  const release = reads.retain(taskId);
  const readGeneration = reads.generation(taskId);
  store.getState().setTaskSessionsLoading(taskId, true);
  try {
    const response = await listTaskSessions(taskId, taskSessionListRequestOptions(signal));
    const sessions = response.sessions ?? [];
    return commitTaskSessionLoad(store, taskId, loadGeneration, {
      sessions,
      force,
      hydrationEpochsAtRequestStart,
      membershipIsCurrent: () => reads.isCurrent(taskId, readGeneration),
    });
  } catch (error) {
    if (!isAbortError(error)) console.error("Failed to load task sessions:", error);
    if (force) throw error;
    return store.getState().taskSessionsByTask.itemsByTaskId[taskId] ?? [];
  } finally {
    release();
    settleTaskSessionLoad(store, taskId, loadGeneration, reads);
  }
}
