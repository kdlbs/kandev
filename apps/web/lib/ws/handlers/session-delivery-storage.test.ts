import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import type { SessionId, TaskId } from "@/lib/types/ids";
import { applyRuntimeStorageHealth } from "./session-delivery-storage";
import { normalizeRuntimeStorageHealth } from "@/lib/session-delivery-storage";

const TASK_ID = "task" as TaskId;
const SESSION_ID = "session" as SessionId;

function seedSession(store: ReturnType<typeof createAppStore>) {
  store.getState().upsertTaskSessionFromEvent(TASK_ID, {
    id: SESSION_ID,
    task_id: TASK_ID,
    state: "RUNNING",
    started_at: "2026-10-27T10:00:00Z",
    updated_at: "2026-10-27T10:00:00Z",
  });
}

function storeWithSession() {
  const store = createAppStore();
  seedSession(store);
  return store;
}

const payload = {
  task_id: TASK_ID,
  session_id: SESSION_ID,
  state: "warning",
  location: "local_runtime",
  total_bytes: 1000,
  available_bytes: 100,
  observed_at: "2026-10-27T10:00:00Z",
  revision: 1,
  epoch: 5,
};

describe("runtime storage WS handler", () => {
  it("applies a warning snapshot and clears it with a newer normal snapshot", () => {
    const store = createAppStore();
    seedSession(store);
    applyRuntimeStorageHealth(store, payload);
    expect(store.getState().taskSessions.items[SESSION_ID]?.runtime_storage_health?.state).toBe(
      "warning",
    );
    applyRuntimeStorageHealth(store, {
      ...payload,
      state: "normal",
      available_bytes: 200,
      revision: 2,
    });
    expect(store.getState().taskSessions.items[SESSION_ID]?.runtime_storage_health?.state).toBe(
      "normal",
    );
  });

  it("rejects stale revisions, malformed states, and unknown sessions", () => {
    const store = createAppStore();
    seedSession(store);
    applyRuntimeStorageHealth(store, { ...payload, revision: 3 });
    applyRuntimeStorageHealth(store, { ...payload, state: "normal", revision: 2 });
    expect(store.getState().taskSessions.items[SESSION_ID]?.runtime_storage_health?.revision).toBe(
      3,
    );
    applyRuntimeStorageHealth(store, { ...payload, state: "invented", revision: 9 });
    expect(store.getState().taskSessions.items[SESSION_ID]?.runtime_storage_health?.revision).toBe(
      3,
    );
    applyRuntimeStorageHealth(store, { ...payload, session_id: "other" });
    expect(store.getState().taskSessions.items[SESSION_ID]?.runtime_storage_health?.revision).toBe(
      3,
    );
  });

  it("accepts a production millisecond epoch and pressure evidence", () => {
    const production = {
      ...payload,
      epoch: Date.now(),
      write_pressure: true,
      failure_storage: { reason: "disk_quota_exceeded", location: "local_runtime" },
    };
    const health = normalizeRuntimeStorageHealth(production);
    expect(health?.epoch).toBe(production.epoch);
    expect(health?.write_pressure).toBe(true);
    expect(health?.failure_storage?.reason).toBe("disk_quota_exceeded");
    const store = storeWithSession();
    applyRuntimeStorageHealth(store, production);
    expect(store.getState().taskSessions.items[SESSION_ID]?.runtime_storage_health?.epoch).toBe(
      production.epoch,
    );
    expect(
      store.getState().taskSessions.items[SESSION_ID]?.runtime_storage_health?.write_pressure,
    ).toBe(true);
  });

  it("normalizes only safe integer measurements", () => {
    expect(normalizeRuntimeStorageHealth(payload)?.total_bytes).toBe(1000);
    expect(
      normalizeRuntimeStorageHealth({ ...payload, available_bytes: -1 })?.available_bytes,
    ).toBe(undefined);
    expect(normalizeRuntimeStorageHealth({ ...payload, revision: undefined })).toBeNull();
    expect(normalizeRuntimeStorageHealth({ ...payload, epoch: 1.5 })).toBeNull();
  });
});
