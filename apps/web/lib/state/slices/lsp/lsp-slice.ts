import type { StateCreator } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { TaskLspCapacity, TaskLspLanguageSnapshot } from "@/lib/types/http-lsp";
import type { LspSlice, LspSliceState, TaskLspTaskState } from "./types";

const EMPTY_CAPACITY: TaskLspCapacity = { active: 0, queued: 0, limit: 0 };

export const defaultLspState: LspSliceState = {
  taskLsp: { byTaskId: {}, pendingByKey: {} },
};

function emptyTaskState(): TaskLspTaskState {
  return {
    languages: {},
    capacity: { ...EMPTY_CAPACITY },
    retiredCapacityEpochs: [],
    loaded: false,
    loading: false,
    error: null,
    errorEpoch: 0,
  };
}

function mergeLanguage(
  languages: Record<string, TaskLspLanguageSnapshot>,
  incoming: TaskLspLanguageSnapshot,
): boolean {
  const current = languages[incoming.language];
  if (current && current.revision > incoming.revision) return false;
  languages[incoming.language] = incoming;
  return true;
}

function mergeCapacity(
  task: TaskLspTaskState,
  incoming: TaskLspCapacity,
  authoritativeEpoch = false,
): void {
  const currentEpoch = task.capacity.epoch;
  if (currentEpoch) {
    if (!incoming.epoch) return;
    if (mergeDifferentCapacityEpoch(task, incoming, authoritativeEpoch)) return;
  } else if (incoming.epoch) {
    task.capacity = incoming;
    return;
  }
  if ((task.capacity.revision ?? 0) > (incoming.revision ?? 0)) return;
  task.capacity = incoming;
}

function mergeDifferentCapacityEpoch(
  task: TaskLspTaskState,
  incoming: TaskLspCapacity,
  authoritative: boolean,
): boolean {
  const currentEpoch = task.capacity.epoch;
  const incomingEpoch = incoming.epoch;
  if (currentEpoch === incomingEpoch) return false;
  if (!currentEpoch || !incomingEpoch) return false;
  if (task.retiredCapacityEpochs?.includes(incomingEpoch) || !authoritative) return true;
  const retired = task.retiredCapacityEpochs ?? [];
  if (!retired.includes(currentEpoch)) task.retiredCapacityEpochs = [...retired, currentEpoch];
  task.capacity = incoming;
  return true;
}

export const createLspSlice: StateCreator<AppState, [["zustand/immer", never]], [], LspSlice> = (
  set,
) => ({
  ...defaultLspState,
  setTaskLspSnapshot: (snapshot, expectedErrorEpoch) =>
    set((draft) => {
      const task = draft.taskLsp.byTaskId[snapshot.task_id] ?? emptyTaskState();
      for (const language of snapshot.languages) mergeLanguage(task.languages, language);
      mergeCapacity(task, snapshot.capacity, true);
      task.loaded = true;
      task.loading = false;
      if (expectedErrorEpoch === undefined || expectedErrorEpoch === (task.errorEpoch ?? 0)) {
        task.error = null;
      }
      draft.taskLsp.byTaskId[snapshot.task_id] = task;
    }),
  mergeTaskLspLanguage: (snapshot) =>
    set((draft) => {
      const task = draft.taskLsp.byTaskId[snapshot.task_id] ?? emptyTaskState();
      const accepted = mergeLanguage(task.languages, snapshot);
      if (snapshot.capacity) mergeCapacity(task, snapshot.capacity);
      if (accepted) task.error = null;
      draft.taskLsp.byTaskId[snapshot.task_id] = task;
    }),
  setTaskLspLoading: (taskId, loading) =>
    set((draft) => {
      const task = draft.taskLsp.byTaskId[taskId] ?? emptyTaskState();
      task.loading = loading;
      draft.taskLsp.byTaskId[taskId] = task;
    }),
  setTaskLspError: (taskId, error) =>
    set((draft) => {
      const task = draft.taskLsp.byTaskId[taskId] ?? emptyTaskState();
      task.error = error;
      if (error !== null) task.errorEpoch = (task.errorEpoch ?? 0) + 1;
      task.loading = false;
      draft.taskLsp.byTaskId[taskId] = task;
    }),
  setTaskLspActionPending: (taskId, language, action) =>
    set((draft) => {
      const key = `${taskId}:${language}`;
      if (action === undefined) delete draft.taskLsp.pendingByKey[key];
      else draft.taskLsp.pendingByKey[key] = action;
    }),
  clearTaskLsp: (taskId) =>
    set((draft) => {
      delete draft.taskLsp.byTaskId[taskId];
      for (const key of Object.keys(draft.taskLsp.pendingByKey)) {
        if (key.startsWith(`${taskId}:`)) delete draft.taskLsp.pendingByKey[key];
      }
    }),
});
