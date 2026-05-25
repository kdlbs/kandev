import type { StateCreator } from "zustand";
import type { SystemSlice, SystemSliceState } from "./types";

export const defaultSystemState: SystemSliceState = {
  system: {
    info: null,
    diskUsage: null,
    database: null,
    backups: { items: [], loaded: false },
    logs: { files: [], tail: [], tailLoaded: false },
    updates: null,
    jobs: {},
  },
};

type ImmerSet = Parameters<
  StateCreator<SystemSlice, [["zustand/immer", never]], [], SystemSlice>
>[0];

export const createSystemSlice: StateCreator<
  SystemSlice,
  [["zustand/immer", never]],
  [],
  SystemSlice
> = (set: ImmerSet, _get, _api) => ({
  ...defaultSystemState,
  setSystemInfo: (info) =>
    set((draft) => {
      draft.system.info = info;
    }),
  setSystemDiskUsage: (usage) =>
    set((draft) => {
      draft.system.diskUsage = usage;
    }),
  setSystemDatabase: (stats) =>
    set((draft) => {
      draft.system.database = stats;
    }),
  setSystemBackups: (items) =>
    set((draft) => {
      draft.system.backups = { items, loaded: true };
    }),
  setSystemLogs: (files) =>
    set((draft) => {
      draft.system.logs.files = files;
    }),
  setSystemLogTail: (lines) =>
    set((draft) => {
      draft.system.logs.tail = lines;
      draft.system.logs.tailLoaded = true;
    }),
  setSystemUpdates: (updates) =>
    set((draft) => {
      draft.system.updates = updates;
    }),
  upsertSystemJob: (job) =>
    set((draft) => {
      draft.system.jobs[job.id] = job;
    }),
  clearSystemJob: (jobId) =>
    set((draft) => {
      delete draft.system.jobs[jobId];
    }),
});
