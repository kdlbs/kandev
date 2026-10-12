import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSystemSlice, defaultSystemState } from "./system-slice";
import type { SystemSlice } from "./types";
import type { UpdatesResponse, SystemJob, StorageOverviewResponse } from "@/lib/types/system";

const TS = "2026-05-18T00:00:00Z";

function makeStore() {
  return create<SystemSlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createSystemSlice as any)(...a) })),
  );
}

const UPDATES: UpdatesResponse = {
  current: "1.2.3",
  latest: "1.2.4",
  latest_url: "https://github.com/kdlbs/kandev/releases/1.2.4",
  latest_checked_at: TS,
  update_available: true,
  channel: "stable",
  channel_editable: true,
  channel_unsupported_reason: "",
};

const JOB: SystemJob = {
  id: "job-1",
  kind: "vacuum",
  state: "running",
  started_at: TS,
};

const ANALYSIS = {
  generation: 1,
  state: "ready",
  started_at: TS,
  completed_at: TS,
  duration_ms: 10,
  cache_ttl_seconds: 900,
  refresh_due_at: "2026-05-18T00:15:00Z",
  stale: false,
  error: null,
  progress: { completed_sources: 8, total_sources: 8, sources: {} },
  partial_summary: null,
} as const;

describe("system storage slice", () => {
  it("stores storage overview, runs, and quarantine state", () => {
    const store = makeStore();
    const overview = {
      settings: {
        enabled: false,
        check_interval_hours: 24,
        idle_for_minutes: 10,
        orphan_grace_hours: 168,
        quarantine_retention_hours: 168,
        workspaces: { enabled: true, dependency_cleanup_enabled: false },
        kandev_containers: { enabled: true },
        go_cache: {
          enabled: false,
          max_bytes: 16106127360,
          adopted_path: "",
          allow_cleanup_while_busy: false,
        },
        docker: {
          dedicated_daemon_acknowledged: false,
          build_cache_enabled: false,
          build_cache_keep_bytes: 10737418240,
          build_cache_unused_hours: 168,
          unused_images_enabled: false,
          unused_images_hours: 168,
        },
      },
      capabilities: {
        managed_go_cache_path: "/data/cache/go-build",
        go_cache_adoption_available: true,
        temporary_artifacts_available: false,
        docker_available: false,
        docker_host: "",
        host_global_docker_cleanup_allowed: false,
      },
      summary: {
        workspaces: { active_bytes: 1, candidate_bytes: 2 },
        go_cache: { path: "/data/cache/go-build", size_bytes: 3, owned: true, enabled: false },
        quarantine: { count: 0, size_bytes: 0 },
        temporary_artifacts: {
          available: false,
          warning: "temporary artifact registry unavailable",
        },
        docker: {
          available: false,
          build_cache_bytes: 0,
          unused_image_bytes: 0,
          managed_container_count: 0,
          managed_container_bytes: 0,
        },
      },
      analysis: ANALYSIS,
      analyzed_at: "2026-07-23T12:00:00Z",
      last_run: null,
    } satisfies StorageOverviewResponse;
    const policy = { settings: overview.settings, capabilities: overview.capabilities };
    store.getState().setSystemStoragePolicy(policy);
    store.getState().setSystemStorageOverview(overview);
    store.getState().setSystemStorageRuns([]);
    store.getState().setSystemStorageQuarantine([]);
    expect(store.getState().system.storage).toEqual({
      policy,
      overview,
      analysisRevision: 0,
      disk: null,
      diskIdentity: null,
      runs: [],
      quarantine: [],
    });
  });
});

describe("system slice", () => {
  it("starts with empty defaults", () => {
    const store = makeStore();
    const s = store.getState();
    expect(s.system).toEqual(defaultSystemState.system);
    expect("diskUsage" in s.system).toBe(false);
    expect("database" in s.system).toBe(false);
    expect("backups" in s.system).toBe(false);
    expect(s.system.updates).toBeNull();
    expect(s.system.jobs).toEqual({});
  });

  it("setSystemRetention stores the status", () => {
    const store = makeStore();
    expect(store.getState().system.retention).toBeNull();
    const status = {
      settings: {
        enabled: true,
        sweep_interval_hours: 6,
        batch_limit: 5000,
        routine_runs: { window_days: 30, floor_per_owner: 50, warn_rows: 25000 },
        runs: { window_days: 30, floor_per_owner: 50, warn_rows: 25000 },
        run_events: { warn_rows: 250000 },
      },
      last_sweep: null,
      skip_count: 0,
      retained_counts: {
        office_routine_runs: { state: "not_computed" as const, retained_count: 0, as_of: "" },
        runs: { state: "not_computed" as const, retained_count: 0, as_of: "" },
        run_events: { state: "not_computed" as const, retained_count: 0, as_of: "" },
      },
    };
    store.getState().setSystemRetention(status);
    expect(store.getState().system.retention).toEqual(status);
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

  it("advances the storage analysis revision for live updates", () => {
    const store = makeStore();
    expect(store.getState().system.storage.analysisRevision).toBe(0);
    store.getState().bumpSystemStorageAnalysisRevision();
    store.getState().bumpSystemStorageAnalysisRevision();
    expect(store.getState().system.storage.analysisRevision).toBe(2);
  });
});
