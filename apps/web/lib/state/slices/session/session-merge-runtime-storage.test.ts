import { describe, expect, it } from "vitest";
import type { RuntimeStorageHealth, TaskSession } from "@/lib/types/http";
import { mergeRuntimeStorageHealth } from "@/lib/session-delivery-storage";
import { mergeTaskSession } from "./session-merge";

const normal: RuntimeStorageHealth = {
  state: "normal",
  location: "local_runtime",
  total_bytes: 1000,
  available_bytes: 500,
  observed_at: "2026-10-27T10:00:00Z",
  age_seconds: 0,
  revision: 2,
  epoch: 5,
};

const warning: RuntimeStorageHealth = {
  ...normal,
  state: "warning",
  available_bytes: 100,
  revision: 1,
};

function session(health: RuntimeStorageHealth | null): TaskSession {
  return {
    id: "session",
    task_id: "task",
    state: "RUNNING",
    started_at: "2026-10-27T10:00:00Z",
    updated_at: "2026-10-27T10:00:00Z",
    runtime_storage_health: health,
  } as TaskSession;
}

describe("runtime storage snapshot merging", () => {
  it("keeps a newer live snapshot when a deferred session snapshot arrives", () => {
    const merged = mergeTaskSession(session(normal), session(warning));
    expect(merged.runtime_storage_health?.state).toBe("normal");
    expect(merged.runtime_storage_health?.revision).toBe(2);
  });

  it("accepts a newer epoch after a backend restart", () => {
    const merged = mergeTaskSession(
      session(normal),
      session({ ...warning, epoch: 6, revision: 1 }),
    );
    expect(merged.runtime_storage_health?.state).toBe("warning");
    expect(merged.runtime_storage_health?.epoch).toBe(6);
  });

  it("preserves live state when a snapshot omits the runtime-only field", () => {
    const incoming = session(null);
    delete incoming.runtime_storage_health;
    const merged = mergeTaskSession(session(normal), incoming);
    expect(merged.runtime_storage_health?.revision).toBe(2);
  });

  it("normalizes wire snapshots before comparison", () => {
    const wire = {
      ...warning,
      epoch: Date.now(),
      write_pressure: true,
      failure_storage: { reason: "disk_quota_exceeded", location: "local_runtime" },
    } as RuntimeStorageHealth;
    const merged = mergeRuntimeStorageHealth(normal, wire);
    expect(merged?.epoch).toBe(wire.epoch);
    expect(merged?.write_pressure).toBe(true);
    expect(merged?.failure_storage?.reason).toBe("disk_quota_exceeded");
  });
});
