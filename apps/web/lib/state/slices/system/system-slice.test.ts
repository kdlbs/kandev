import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSystemSlice, defaultSystemState } from "./system-slice";
import type { SystemSlice } from "./types";
import type {
  SystemInfo,
  DiskUsageResponse,
  DatabaseStats,
  SnapshotInfo,
  LogFileInfo,
  UpdatesResponse,
  SystemJob,
} from "@/lib/types/system";

const TS = "2026-05-18T00:00:00Z";

function makeStore() {
  return create<SystemSlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createSystemSlice as any)(...a) })),
  );
}

const INFO: SystemInfo = {
  version: "1.2.3",
  commit: "abc1234",
  build_time: "2026-01-01T00:00:00Z",
  go_version: "go1.24",
  os: "darwin",
  arch: "arm64",
};

const DISK_USAGE: DiskUsageResponse = {
  data: {
    data_dir: 100,
    worktrees: 200,
    repos: 300,
    sessions: 400,
    tasks: 500,
    quick_chat: 600,
    backups: 700,
    total: 2800,
    warnings: [],
    computed_at: TS,
  },
  computing: false,
};

const DB_STATS: DatabaseStats = {
  path: "/data/kandev.db",
  size_bytes: 12345,
  wal_size_bytes: 678,
  schema_version: "1.0.0",
  last_backup_at: "2026-05-17T00:00:00Z",
};

const SNAPSHOT: SnapshotInfo = {
  name: "manual-1.db",
  path: "/data/backups/manual-1.db",
  size_bytes: 1024,
  mtime: "2026-05-17T00:00:00Z",
  kind: "manual",
};

const LOG_FILE: LogFileInfo = {
  name: "kandev.log",
  size: 2048,
  mtime: TS,
  current: true,
};

const UPDATES: UpdatesResponse = {
  current: "1.2.3",
  latest: "1.2.4",
  latest_url: "https://github.com/kdlbs/kandev/releases/1.2.4",
  latest_checked_at: TS,
  update_available: true,
};

const JOB: SystemJob = {
  id: "job-1",
  kind: "vacuum",
  state: "running",
  started_at: TS,
};

describe("system slice", () => {
  it("starts with empty defaults", () => {
    const store = makeStore();
    const s = store.getState();
    expect(s.system).toEqual(defaultSystemState.system);
    expect(s.system.info).toBeNull();
    expect(s.system.diskUsage).toBeNull();
    expect(s.system.database).toBeNull();
    expect(s.system.backups).toEqual({ items: [], loaded: false });
    expect(s.system.logs).toEqual({ files: [], tail: [], tailLoaded: false });
    expect(s.system.updates).toBeNull();
    expect(s.system.jobs).toEqual({});
  });

  it("setSystemInfo stores the payload", () => {
    const store = makeStore();
    store.getState().setSystemInfo(INFO);
    expect(store.getState().system.info).toEqual(INFO);
  });

  it("setSystemDiskUsage replaces the cached response", () => {
    const store = makeStore();
    store.getState().setSystemDiskUsage(DISK_USAGE);
    expect(store.getState().system.diskUsage).toEqual(DISK_USAGE);

    const computing: DiskUsageResponse = { data: null, computing: true };
    store.getState().setSystemDiskUsage(computing);
    expect(store.getState().system.diskUsage).toEqual(computing);
  });

  it("setSystemDatabase stores the stats", () => {
    const store = makeStore();
    store.getState().setSystemDatabase(DB_STATS);
    expect(store.getState().system.database).toEqual(DB_STATS);
  });

  it("setSystemBackups marks the list as loaded", () => {
    const store = makeStore();
    store.getState().setSystemBackups([SNAPSHOT]);
    expect(store.getState().system.backups).toEqual({ items: [SNAPSHOT], loaded: true });

    // Empty list also flips loaded to true.
    store.getState().setSystemBackups([]);
    expect(store.getState().system.backups).toEqual({ items: [], loaded: true });
  });

  it("setSystemLogs replaces only the files (tail stays untouched)", () => {
    const store = makeStore();
    store.getState().setSystemLogTail(["line 1", "line 2"]);
    store.getState().setSystemLogs([LOG_FILE]);
    expect(store.getState().system.logs.files).toEqual([LOG_FILE]);
    expect(store.getState().system.logs.tail).toEqual(["line 1", "line 2"]);
    expect(store.getState().system.logs.tailLoaded).toBe(true);
  });

  it("setSystemLogTail flips tailLoaded to true", () => {
    const store = makeStore();
    expect(store.getState().system.logs.tailLoaded).toBe(false);
    store.getState().setSystemLogTail(["hello"]);
    expect(store.getState().system.logs.tail).toEqual(["hello"]);
    expect(store.getState().system.logs.tailLoaded).toBe(true);
  });

  it("setSystemUpdates stores the response", () => {
    const store = makeStore();
    store.getState().setSystemUpdates(UPDATES);
    expect(store.getState().system.updates).toEqual(UPDATES);
  });

  it("upsertSystemJob inserts and updates by id", () => {
    const store = makeStore();
    store.getState().upsertSystemJob(JOB);
    expect(store.getState().system.jobs["job-1"]).toEqual(JOB);

    const finished: SystemJob = { ...JOB, state: "succeeded", ended_at: "2026-05-18T00:01:00Z" };
    store.getState().upsertSystemJob(finished);
    expect(store.getState().system.jobs["job-1"]).toEqual(finished);
    // Same id, still one entry.
    expect(Object.keys(store.getState().system.jobs)).toEqual(["job-1"]);
  });

  it("clearSystemJob removes the entry", () => {
    const store = makeStore();
    store.getState().upsertSystemJob(JOB);
    store.getState().upsertSystemJob({ ...JOB, id: "job-2" });
    store.getState().clearSystemJob("job-1");
    expect(store.getState().system.jobs["job-1"]).toBeUndefined();
    expect(store.getState().system.jobs["job-2"]).toBeDefined();
  });

  it("clearSystemJob is a no-op for missing ids", () => {
    const store = makeStore();
    store.getState().clearSystemJob("does-not-exist");
    expect(store.getState().system.jobs).toEqual({});
  });
});
