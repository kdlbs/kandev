import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { registerKanbanBridge, applyIfNewer } from "../kanban";
import { qk } from "@/lib/query/keys";
import type { KanbanMultiData } from "@/lib/query/query-options/kanban";
import type { KanbanState } from "@/lib/state/slices/kanban/types";

// ---------------------------------------------------------------------------
// Shared constants
// ---------------------------------------------------------------------------

const WF1 = "wf-1";
const WF2 = "wf-2";
const TS_OLD = "2024-01-01T00:00:00Z";
const TS_NEW = "2024-01-02T00:00:00Z";

// WS event type name constants (used in ws.on subscriptions and ws.emit calls)
const EV_KANBAN_UPDATE = "kanban.update";
const EV_TASK_CREATED = "task.created";
const EV_TASK_UPDATED = "task.updated";
const EV_TASK_DELETED = "task.deleted";
const EV_TASK_STATE_CHANGED = "task.state_changed";

// Step fixture constants
const STEP_COLOR = "bg-red-400";

// ---------------------------------------------------------------------------
// Fake WS helpers
// ---------------------------------------------------------------------------

type Handler = (message: { payload: unknown }) => void;
interface FakeWs {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  on: ReturnType<typeof vi.fn<any[], any>>;
  emit: (type: string, payload: unknown) => void;
  listeners: Map<string, Set<Handler>>;
}

function createFakeWs(): FakeWs {
  const listeners = new Map<string, Set<Handler>>();

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const on: ReturnType<typeof vi.fn<any[], any>> = vi.fn((type: string, handler: Handler) => {
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

  return { on, emit, listeners };
}

function createTestClient(): QueryClient {
  return new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
}

function makeTask(overrides: Partial<KanbanState["tasks"][number]> = {}): KanbanState["tasks"][number] {
  return {
    id: "t1",
    workflowStepId: "s1",
    title: "Task 1",
    position: 0,
    ...overrides,
  } as KanbanState["tasks"][number];
}

function seedMulti(qc: QueryClient, data: KanbanMultiData) {
  qc.setQueryData<KanbanMultiData>(qk.kanban.multi(), data);
}

function getMulti(qc: QueryClient): KanbanMultiData | undefined {
  return qc.getQueryData<KanbanMultiData>(qk.kanban.multi());
}

// ---------------------------------------------------------------------------
// applyIfNewer unit tests
// ---------------------------------------------------------------------------

describe("applyIfNewer", () => {
  it("returns next when prev is undefined", () => {
    const next = makeTask({ id: "t1", updatedAt: TS_NEW });
    expect(applyIfNewer(undefined, next)).toBe(next);
  });

  it("returns next when next is strictly newer", () => {
    const prev = makeTask({ updatedAt: TS_OLD });
    const next = makeTask({ updatedAt: TS_NEW, title: "Updated" });
    expect(applyIfNewer(prev, next)).toBe(next);
  });

  it("returns prev when next has the same timestamp", () => {
    const prev = makeTask({ updatedAt: TS_NEW, title: "Prev" });
    const next = makeTask({ updatedAt: TS_NEW, title: "Next" });
    expect(applyIfNewer(prev, next)).toBe(prev);
  });

  it("returns prev when next is older", () => {
    const prev = makeTask({ updatedAt: TS_NEW });
    const next = makeTask({ updatedAt: TS_OLD, title: "Stale" });
    expect(applyIfNewer(prev, next)).toBe(prev);
  });

  it("returns next when neither has updatedAt", () => {
    const prev = makeTask({ updatedAt: undefined });
    const next = makeTask({ updatedAt: undefined, title: "Next" });
    expect(applyIfNewer(prev, next)).toBe(next);
  });

  it("returns prev when next has no updatedAt but prev does", () => {
    const prev = makeTask({ updatedAt: TS_NEW, title: "Prev" });
    const next = makeTask({ updatedAt: undefined, title: "Next" });
    expect(applyIfNewer(prev, next)).toBe(prev);
  });
});

// ---------------------------------------------------------------------------
// Bridge registration
// ---------------------------------------------------------------------------

describe("registerKanbanBridge — registration", () => {
  it("returns a cleanup function", () => {
    const qc = createTestClient();
    const ws = createFakeWs();
    const cleanup = registerKanbanBridge(ws as never, qc);
    expect(typeof cleanup).toBe("function");
    cleanup();
  });

  it("subscribes to expected WS event types", () => {
    const qc = createTestClient();
    const ws = createFakeWs();
    registerKanbanBridge(ws as never, qc);
    const types = (ws.on as ReturnType<typeof vi.fn>).mock.calls.map((c) => c[0] as string);
    expect(types).toContain(EV_KANBAN_UPDATE);
    expect(types).toContain(EV_TASK_CREATED);
    expect(types).toContain(EV_TASK_UPDATED);
    expect(types).toContain(EV_TASK_DELETED);
    expect(types).toContain(EV_TASK_STATE_CHANGED);
  });
});

// ---------------------------------------------------------------------------
// kanban.update handler
// ---------------------------------------------------------------------------

describe("registerKanbanBridge — kanban.update", () => {
  let ws: FakeWs;
  let qc: QueryClient;

  beforeEach(() => {
    ws = createFakeWs();
    qc = createTestClient();
    registerKanbanBridge(ws as never, qc);
  });

  it("seeds cache with snapshot data", () => {
    ws.emit(EV_KANBAN_UPDATE, {
      workflowId: WF1,
      steps: [{ id: "s1", title: "Step 1", color: STEP_COLOR, position: 0 }],
      tasks: [{ id: "t1", workflowStepId: "s1", title: "T1", position: 0 }],
    });
    const data = getMulti(qc);
    expect(data?.snapshots[WF1]?.tasks).toHaveLength(1);
    expect(data?.snapshots[WF1]?.steps).toHaveLength(1);
  });

  it("filters out ephemeral tasks", () => {
    ws.emit(EV_KANBAN_UPDATE, {
      workflowId: WF1,
      steps: [],
      tasks: [
        { id: "t-real", workflowStepId: "s1", title: "Real", position: 0 },
        { id: "t-ephem", workflowStepId: "s1", title: "Ephemeral", position: 1, is_ephemeral: true },
      ],
    });
    const tasks = getMulti(qc)?.snapshots[WF1]?.tasks ?? [];
    expect(tasks.map((t) => t.id)).toEqual(["t-real"]);
  });

  it("preserves primarySessionId from existing cache (undefined → fallback)", () => {
    seedMulti(qc, {
      snapshots: {
        [WF1]: {
          workflowId: WF1,
          workflowName: "WF1",
          steps: [],
          tasks: [makeTask({ id: "t1", primarySessionId: "sess-old" })],
        },
      },
    });

    ws.emit(EV_KANBAN_UPDATE, {
      workflowId: WF1,
      steps: [],
      tasks: [{ id: "t1", workflowStepId: "s1", title: "Updated", position: 0 }],
    });

    const task = getMulti(qc)?.snapshots[WF1]?.tasks.find((t) => t.id === "t1");
    expect(task?.primarySessionId).toBe("sess-old");
    expect(task?.title).toBe("Updated");
  });

  it("does NOT restore stale primarySessionId when event explicitly clears it (null sentinel)", () => {
    seedMulti(qc, {
      snapshots: {
        [WF1]: {
          workflowId: WF1,
          workflowName: "WF1",
          steps: [],
          tasks: [makeTask({ id: "t1", primarySessionId: "stale-sess" })],
        },
      },
    });

    ws.emit(EV_KANBAN_UPDATE, {
      workflowId: WF1,
      steps: [],
      tasks: [{ id: "t1", workflowStepId: "s1", title: "T1", position: 0, primarySessionId: null }],
    });

    const task = getMulti(qc)?.snapshots[WF1]?.tasks.find((t) => t.id === "t1");
    expect(task?.primarySessionId).toBeNull();
  });

  it("does not affect other workflows in the cache", () => {
    seedMulti(qc, {
      snapshots: {
        [WF2]: {
          workflowId: WF2,
          workflowName: "WF2",
          steps: [],
          tasks: [makeTask({ id: "t2" })],
        },
      },
    });

    ws.emit(EV_KANBAN_UPDATE, {
      workflowId: WF1,
      steps: [],
      tasks: [makeTask({ id: "t1" })],
    });

    expect(getMulti(qc)?.snapshots[WF2]?.tasks).toHaveLength(1);
  });
});

// ---------------------------------------------------------------------------
// task.created / task.updated / task.deleted / task.state_changed handlers
// ---------------------------------------------------------------------------

function seedTaskEventState(qc: QueryClient) {
  seedMulti(qc, {
    snapshots: {
      [WF1]: {
        workflowId: WF1,
        workflowName: "WF1",
        steps: [],
        tasks: [makeTask({ id: "t1", updatedAt: TS_OLD })],
      },
    },
  });
}

describe("registerKanbanBridge — task.created / task.deleted / task.state_changed", () => {
  let ws: FakeWs;
  let qc: QueryClient;

  beforeEach(() => {
    ws = createFakeWs();
    qc = createTestClient();
    registerKanbanBridge(ws as never, qc);
    seedTaskEventState(qc);
  });

  it("task.created appends a new task", () => {
    ws.emit(EV_TASK_CREATED, {
      id: "t2",
      task_id: "t2",
      workflow_id: WF1,
      workflow_step_id: "s1",
      title: "New",
      position: 1,
    });
    const tasks = getMulti(qc)?.snapshots[WF1]?.tasks ?? [];
    expect(tasks.map((t) => t.id)).toContain("t2");
  });

  it("task.created skips ephemeral tasks", () => {
    ws.emit(EV_TASK_CREATED, {
      id: "t-ephem",
      task_id: "t-ephem",
      workflow_id: WF1,
      workflow_step_id: "s1",
      title: "Quick chat",
      position: 0,
      is_ephemeral: true,
    });
    const tasks = getMulti(qc)?.snapshots[WF1]?.tasks ?? [];
    expect(tasks.map((t) => t.id)).not.toContain("t-ephem");
  });

  it("task.deleted removes task from workflow", () => {
    ws.emit(EV_TASK_DELETED, { task_id: "t1", workflow_id: WF1 });
    const tasks = getMulti(qc)?.snapshots[WF1]?.tasks ?? [];
    expect(tasks.map((t) => t.id)).not.toContain("t1");
  });

  it("task.state_changed upserts task", () => {
    ws.emit(EV_TASK_STATE_CHANGED, {
      task_id: "t1",
      workflow_id: WF1,
      workflow_step_id: "s1",
      title: "Task 1",
      position: 0,
      state: "IN_PROGRESS",
      updated_at: TS_NEW,
    });
    const task = getMulti(qc)?.snapshots[WF1]?.tasks.find((t) => t.id === "t1");
    expect(task?.state).toBe("IN_PROGRESS");
  });
});

describe("registerKanbanBridge — task.updated", () => {
  let ws: FakeWs;
  let qc: QueryClient;

  beforeEach(() => {
    ws = createFakeWs();
    qc = createTestClient();
    registerKanbanBridge(ws as never, qc);
    seedTaskEventState(qc);
  });

  it("upserts task with timestamp guard (newer wins)", () => {
    ws.emit(EV_TASK_UPDATED, {
      task_id: "t1",
      workflow_id: WF1,
      workflow_step_id: "s1",
      title: "Newer Title",
      position: 0,
      updated_at: TS_NEW,
    });
    const task = getMulti(qc)?.snapshots[WF1]?.tasks.find((t) => t.id === "t1");
    expect(task?.title).toBe("Newer Title");
  });

  it("ignores stale event (older timestamp)", () => {
    ws.emit(EV_TASK_UPDATED, {
      task_id: "t1",
      workflow_id: WF1,
      workflow_step_id: "s1",
      title: "Stale Title",
      position: 0,
      updated_at: TS_OLD,
    });
    const task = getMulti(qc)?.snapshots[WF1]?.tasks.find((t) => t.id === "t1");
    // TS_OLD <= TS_OLD → stale event, original title preserved
    expect(task?.title).toBe("Task 1");
  });

  it("removes task when archived_at is set", () => {
    ws.emit(EV_TASK_UPDATED, {
      task_id: "t1",
      workflow_id: WF1,
      updated_at: TS_NEW,
      archived_at: TS_NEW,
    });
    const tasks = getMulti(qc)?.snapshots[WF1]?.tasks ?? [];
    expect(tasks.map((t) => t.id)).not.toContain("t1");
  });

  it("removes from old workflow and upserts in new workflow", () => {
    seedMulti(qc, {
      snapshots: {
        [WF1]: { workflowId: WF1, workflowName: "WF1", steps: [], tasks: [makeTask({ id: "t1" })] },
        [WF2]: { workflowId: WF2, workflowName: "WF2", steps: [], tasks: [] },
      },
    });

    ws.emit(EV_TASK_UPDATED, {
      task_id: "t1",
      workflow_id: WF2,
      old_workflow_id: WF1,
      workflow_step_id: "s1",
      title: "Moved",
      position: 0,
      updated_at: TS_NEW,
    });

    expect(getMulti(qc)?.snapshots[WF1]?.tasks.map((t) => t.id)).not.toContain("t1");
    expect(getMulti(qc)?.snapshots[WF2]?.tasks.map((t) => t.id)).toContain("t1");
  });
});

// ---------------------------------------------------------------------------
// Workflow event handlers
// ---------------------------------------------------------------------------

describe("registerKanbanBridge — workflow events", () => {
  let ws: FakeWs;
  let qc: QueryClient;

  beforeEach(() => {
    ws = createFakeWs();
    qc = createTestClient();
    registerKanbanBridge(ws as never, qc);
    seedMulti(qc, {
      snapshots: {
        [WF1]: {
          workflowId: WF1,
          workflowName: "WF1",
          steps: [{ id: "s1", title: "Step 1", color: STEP_COLOR, position: 0 }],
          tasks: [],
        },
      },
    });
  });

  it("workflow.updated patches the workflow name", () => {
    ws.emit("workflow.updated", { id: WF1, name: "WF1 Renamed", hidden: false });
    expect(getMulti(qc)?.snapshots[WF1]?.workflowName).toBe("WF1 Renamed");
  });

  it("workflow.deleted removes the snapshot", () => {
    ws.emit("workflow.deleted", { id: WF1 });
    expect(getMulti(qc)?.snapshots[WF1]).toBeUndefined();
  });

  it("workflow.step.created adds a step", () => {
    ws.emit("workflow.step.created", {
      step: { id: "s2", title: "Step 2", name: "Step 2", color: "bg-blue-400", position: 1, workflow_id: WF1 },
    });
    expect(getMulti(qc)?.snapshots[WF1]?.steps.map((s) => s.id)).toContain("s2");
  });

  it("workflow.step.created ignores duplicate step id", () => {
    ws.emit("workflow.step.created", {
      step: { id: "s1", title: "Dup", name: "Dup", color: STEP_COLOR, position: 0, workflow_id: WF1 },
    });
    expect(getMulti(qc)?.snapshots[WF1]?.steps).toHaveLength(1);
  });

  it("workflow.step.updated patches the step", () => {
    ws.emit("workflow.step.updated", {
      step: { id: "s1", title: "Step 1 Updated", name: "Step 1 Updated", color: STEP_COLOR, position: 0, workflow_id: WF1 },
    });
    expect(getMulti(qc)?.snapshots[WF1]?.steps[0].title).toBe("Step 1 Updated");
  });

  it("workflow.step.deleted removes the step", () => {
    ws.emit("workflow.step.deleted", { step: { id: "s1", workflow_id: WF1 } });
    expect(getMulti(qc)?.snapshots[WF1]?.steps.map((s) => s.id)).not.toContain("s1");
  });
});

// ---------------------------------------------------------------------------
// Cleanup
// ---------------------------------------------------------------------------

describe("registerKanbanBridge — cleanup", () => {
  it("unregisters handlers so subsequent events do not mutate the cache", () => {
    const ws = createFakeWs();
    const qc = createTestClient();
    const cleanup = registerKanbanBridge(ws as never, qc);
    seedMulti(qc, {
      snapshots: {
        [WF1]: { workflowId: WF1, workflowName: "WF1", steps: [], tasks: [] },
      },
    });

    cleanup();

    ws.emit(EV_TASK_CREATED, {
      id: "t-after",
      task_id: "t-after",
      workflow_id: WF1,
      workflow_step_id: "s1",
      title: "After cleanup",
      position: 0,
    });
    const tasks = getMulti(qc)?.snapshots[WF1]?.tasks ?? [];
    expect(tasks.map((t) => t.id)).not.toContain("t-after");
  });
});
