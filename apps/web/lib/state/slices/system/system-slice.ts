import type { StateCreator } from "zustand";
import type { SystemSlice, SystemSliceState } from "./types";

export const defaultSystemState: SystemSliceState = {
  system: {
    info: null,
    diskUsage: null,
    database: null,
    backups: { items: [], loaded: false },
    updates: null,
    jobs: {},
    metrics: null,
    storage: { policy: null, overview: null, disk: null, runs: [], quarantine: [] },
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
  setSystemMetricsSnapshot: (snapshot) =>
    set((draft) => {
      draft.system.metrics = snapshot;
    }),
  setSystemStoragePolicy: (policy) =>
    set((draft) => {
      draft.system.storage.policy = policy;
    }),
  setSystemStorageOverview: (overview) =>
    set((draft) => {
      draft.system.storage.overview = overview;
    }),
  setSystemStorageDisk: (disk) =>
    set((draft) => {
      draft.system.storage.disk = disk;
    }),
  setSystemStorageRuns: (runs) =>
    set((draft) => {
      draft.system.storage.runs = runs;
    }),
  setSystemStorageQuarantine: (entries) =>
    set((draft) => {
      draft.system.storage.quarantine = entries;
    }),
});
