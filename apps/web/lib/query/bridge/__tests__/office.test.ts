import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { registerOfficeBridge } from "../office";
import { qk } from "@/lib/query/keys";
import type { ProviderHealth, RouteAttempt, OfficeTask } from "@/lib/state/slices/office/types";
import type { WebSocketClient } from "@/lib/ws/client";

// ---------------------------------------------------------------------------
// Fake WS helpers (same pattern as kanban test)
// ---------------------------------------------------------------------------

type Handler = (message: { payload: unknown }) => void;
type OnFn = (type: string, handler: Handler) => () => void;
interface FakeWs {
  on: ReturnType<typeof vi.fn<Parameters<OnFn>, ReturnType<OnFn>>>;
  emit: (type: string, payload: unknown) => void;
}

function createFakeWs(): FakeWs {
  const listeners = new Map<string, Set<Handler>>();

  const on = vi.fn((type: string, handler: Handler): (() => void) => {
    const set = listeners.get(type) ?? new Set();
    set.add(handler);
    listeners.set(type, set);
    return () => {
      listeners.get(type)?.delete(handler);
    };
  });

  function emit(type: string, payload: unknown) {
    for (const h of listeners.get(type) ?? []) {
      h({ payload } as { payload: unknown });
    }
  }

  return { on, emit };
}

function bridge(ws: FakeWs, qc: QueryClient): () => void {
  return registerOfficeBridge(ws as unknown as WebSocketClient, qc, () => ACTIVE_WS_ID);
}

function createTestClient(): QueryClient {
  return new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
}

const WS_ID = "ws-123";
const ACTIVE_WS_ID = WS_ID;
const TASK_ID = "task-1";
const PROVIDER_ID = "claude-acp";
const RUN_ID = "run-1";
const STARTED_AT = "2026-01-01T00:00:00Z";

function makeTask(overrides: Partial<OfficeTask> = {}): OfficeTask {
  return {
    id: TASK_ID,
    workspaceId: WS_ID,
    identifier: "OFC-1",
    title: "Test Task",
    status: "todo",
    priority: "medium",
    createdAt: STARTED_AT,
    updatedAt: STARTED_AT,
    ...overrides,
  };
}

function makeHealth(overrides: Partial<ProviderHealth> = {}): ProviderHealth {
  return {
    provider_id: PROVIDER_ID,
    scope: "provider",
    scope_value: PROVIDER_ID,
    state: "healthy",
    backoff_step: 0,
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("registerOfficeBridge — task handlers", () => {
  let qc: QueryClient;
  let ws: FakeWs;

  beforeEach(() => {
    qc = createTestClient();
    ws = createFakeWs();
    // Seed tasks cache
    qc.setQueryData(qk.office.tasks(WS_ID), [makeTask()]);
    bridge(ws, qc);
  });

  it("office.task.updated patches the task in cache", () => {
    ws.emit("office.task.updated", {
      workspace_id: WS_ID,
      task_id: TASK_ID,
      title: "Updated Title",
      status: "in_progress",
    });

    const tasks = qc.getQueryData<OfficeTask[]>(qk.office.tasks(WS_ID));
    expect(tasks?.[0]?.title).toBe("Updated Title");
    expect(tasks?.[0]?.status).toBe("in_progress");
  });

  it("office.task.updated ignores cross-workspace events", () => {
    ws.emit("office.task.updated", {
      workspace_id: "other-ws",
      task_id: TASK_ID,
      title: "Should Not Update",
    });

    const tasks = qc.getQueryData<OfficeTask[]>(qk.office.tasks(WS_ID));
    expect(tasks?.[0]?.title).toBe("Test Task");
  });

  it("office.task.moved patches status in cache", () => {
    ws.emit("office.task.moved", {
      workspace_id: WS_ID,
      task_id: TASK_ID,
      new_status: "done",
    });

    const tasks = qc.getQueryData<OfficeTask[]>(qk.office.tasks(WS_ID));
    expect(tasks?.[0]?.status).toBe("done");
  });

  it("office.task.status_changed patches status in cache", () => {
    ws.emit("office.task.status_changed", {
      workspace_id: WS_ID,
      task_id: TASK_ID,
      new_status: "in_review",
    });

    const tasks = qc.getQueryData<OfficeTask[]>(qk.office.tasks(WS_ID));
    expect(tasks?.[0]?.status).toBe("in_review");
  });
});

describe("registerOfficeBridge — routing handlers", () => {
  let qc: QueryClient;
  let ws: FakeWs;
  const RUN_ATTEMPTS_KEY = ["office", "runs", RUN_ID, "attempts"] as const;

  beforeEach(() => {
    qc = createTestClient();
    ws = createFakeWs();
    bridge(ws, qc);
  });

  it("office.provider.health_changed upserts a new health row", () => {
    const health = makeHealth({ state: "degraded" });
    ws.emit("office.provider.health_changed", {
      workspace_id: WS_ID,
      provider_id: health.provider_id,
      scope: health.scope,
      scope_value: health.scope_value,
      state: health.state,
      backoff_step: health.backoff_step,
    });

    const cached = qc.getQueryData<ProviderHealth[]>(qk.office.providerHealth(WS_ID));
    expect(cached).toHaveLength(1);
    expect(cached?.[0]?.state).toBe("degraded");
  });

  it("office.provider.health_changed updates an existing row in place", () => {
    qc.setQueryData(qk.office.providerHealth(WS_ID), [makeHealth({ state: "healthy" })]);

    ws.emit("office.provider.health_changed", {
      workspace_id: WS_ID,
      provider_id: PROVIDER_ID,
      scope: "provider",
      scope_value: PROVIDER_ID,
      state: "degraded",
      backoff_step: 1,
    });

    const cached = qc.getQueryData<ProviderHealth[]>(qk.office.providerHealth(WS_ID));
    expect(cached).toHaveLength(1);
    expect(cached?.[0]?.state).toBe("degraded");
    expect(cached?.[0]?.backoff_step).toBe(1);
  });

  it("office.route_attempt.appended upserts a run attempt by seq", () => {
    const attempt: RouteAttempt = {
      seq: 1,
      provider_id: PROVIDER_ID,
      tier: "frontier",
      outcome: "launched",
      started_at: STARTED_AT,
    };
    ws.emit("office.route_attempt.appended", {
      workspace_id: WS_ID,
      run_id: RUN_ID,
      attempt,
    });

    const cached = qc.getQueryData<RouteAttempt[]>(RUN_ATTEMPTS_KEY);
    expect(cached).toHaveLength(1);
    expect(cached?.[0]?.seq).toBe(1);
  });

  it("office.route_attempt.appended updates existing attempt with same seq", () => {
    const existing: RouteAttempt = {
      seq: 1,
      provider_id: PROVIDER_ID,
      tier: "frontier",
      outcome: "launched",
      started_at: STARTED_AT,
    };
    qc.setQueryData(RUN_ATTEMPTS_KEY, [existing]);

    ws.emit("office.route_attempt.appended", {
      workspace_id: WS_ID,
      run_id: RUN_ID,
      attempt: { ...existing, outcome: "failed_other" },
    });

    const cached = qc.getQueryData<RouteAttempt[]>(RUN_ATTEMPTS_KEY);
    expect(cached).toHaveLength(1);
    expect(cached?.[0]?.outcome).toBe("failed_other");
  });
});

describe("registerOfficeBridge — cleanup", () => {
  it("unregisters all handlers on cleanup", () => {
    const qc = createTestClient();
    const ws = createFakeWs();
    const cleanup = bridge(ws, qc);
    const registeredCount = ws.on.mock.calls.length;
    expect(registeredCount).toBeGreaterThan(0);

    cleanup();
    // Seed tasks and emit — should have no effect after cleanup.
    qc.setQueryData(qk.office.tasks(WS_ID), [makeTask()]);
    ws.emit("office.task.updated", {
      workspace_id: WS_ID,
      task_id: TASK_ID,
      title: "Post-Cleanup Update",
    });
    const tasks = qc.getQueryData<OfficeTask[]>(qk.office.tasks(WS_ID));
    expect(tasks?.[0]?.title).toBe("Test Task");
  });
});

describe("registerOfficeBridge — select-pattern (sort/group derivation)", () => {
  it("tasks cache is raw array; consumers sort via select without invalidation", () => {
    const qc = createTestClient();
    const ws = createFakeWs();
    bridge(ws, qc);

    const unsortedTasks = [
      makeTask({ id: "t1", priority: "low", updatedAt: STARTED_AT }),
      makeTask({ id: "t2", priority: "high", updatedAt: "2026-01-02T00:00:00Z" }),
    ];
    qc.setQueryData(qk.office.tasks(WS_ID), unsortedTasks);

    // Simulate sort/group happening in select (outside the cache).
    const raw = qc.getQueryData<OfficeTask[]>(qk.office.tasks(WS_ID)) ?? [];
    const sortedByPriority = [...raw].sort((a, b) => (a.priority < b.priority ? -1 : 1));
    expect(sortedByPriority[0]?.id).toBe("t2"); // high < low alphabetically
    // Cache itself remains unsorted — sort is view-only.
    const cacheAfter = qc.getQueryData<OfficeTask[]>(qk.office.tasks(WS_ID));
    expect(cacheAfter?.[0]?.id).toBe("t1");
  });
});
