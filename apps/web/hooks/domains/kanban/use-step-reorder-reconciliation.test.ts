import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";

// use-step-reorder.test.ts already sits near the file-length/function-length
// limit, so the AC.27 in-flight reconciliation coverage gets its own file —
// same split rationale as lib/ws/handlers/task-reordered.test.ts.

const mockToast = vi.fn();
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: mockToast }) }));

const reorderStepTasks = vi.fn();
vi.mock("@/lib/api/domains/kanban-api", () => ({
  reorderStepTasks: (...args: unknown[]) => reorderStepTasks(...args),
}));

type FakeTask = {
  id: string;
  workflowStepId: string;
  position: number;
  wipAdmitted?: boolean;
  queuedForStepId?: string;
};

type WithheldEntry = { revision: number; tasks: Array<{ id: string; position: number }> };

let storeState: {
  kanbanMulti: {
    snapshots: Record<string, { tasks: FakeTask[] }>;
    pendingReorderBandKeys: Record<string, true>;
    orderRevisionByStepId: Record<string, number>;
    withheldReorderByBandKey: Record<string, WithheldEntry | undefined>;
  };
  setWorkflowSnapshot: (wfId: string, snapshot: { tasks: FakeTask[] }) => void;
  setBandReorderPending: (stepId: string, band: string, pending: boolean) => void;
  setStepOrderRevision: (stepId: string, revision: number) => void;
  setWithheldReorder: (stepId: string, band: string, payload: WithheldEntry | null) => void;
};

const WORKFLOW_ID = "wf1";
const STEP_ID = "step-1";

function admittedTask(id: string, position: number): FakeTask {
  return { id, workflowStepId: STEP_ID, position, wipAdmitted: true };
}

function queuedTask(id: string, position: number): FakeTask {
  return { id, workflowStepId: STEP_ID, position, wipAdmitted: false, queuedForStepId: STEP_ID };
}

function resetStore(tasks: FakeTask[]) {
  storeState = {
    kanbanMulti: {
      snapshots: { [WORKFLOW_ID]: { tasks } },
      pendingReorderBandKeys: {},
      orderRevisionByStepId: {},
      withheldReorderByBandKey: {},
    },
    setWorkflowSnapshot(wfId, snapshot) {
      storeState.kanbanMulti.snapshots[wfId] = snapshot;
    },
    setBandReorderPending(stepId, band, pending) {
      const key = `${stepId}:${band}`;
      if (pending) storeState.kanbanMulti.pendingReorderBandKeys[key] = true;
      else delete storeState.kanbanMulti.pendingReorderBandKeys[key];
    },
    setStepOrderRevision(stepId, revision) {
      storeState.kanbanMulti.orderRevisionByStepId[stepId] = revision;
    },
    setWithheldReorder(stepId, band, payload) {
      const key = `${stepId}:${band}`;
      if (payload) storeState.kanbanMulti.withheldReorderByBandKey[key] = payload;
      else delete storeState.kanbanMulti.withheldReorderByBandKey[key];
    },
  };
}

const store = { getState: () => storeState };
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => store,
  useAppStore: (selector: (s: typeof storeState) => unknown) => selector(storeState),
}));

import { useStepReorder } from "./use-step-reorder";

beforeEach(() => {
  resetStore([admittedTask("a", 0), admittedTask("b", 1), admittedTask("c", 2)]);
  reorderStepTasks.mockReset();
  mockToast.mockReset();
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe("useStepReorder — in-flight reconciliation (AC.27), own-response paths", () => {
  it("applies the withheld order instead of a stale own-response when it carries a higher revision", async () => {
    const { promise, resolve } = deferred<{
      workflow_step_id: string;
      revision: number;
      tasks: Array<{ id: string; position: number }>;
    }>();
    reorderStepTasks.mockReturnValue(promise);
    const { result } = renderHook(() => useStepReorder());

    let pending!: Promise<void>;
    act(() => {
      pending = result.current.reorderBand({
        workflowId: WORKFLOW_ID,
        stepId: STEP_ID,
        band: "admitted",
        draggedId: "c",
        visibleOrderAfterMove: ["c", "a", "b"],
      });
    });

    // Simulates a WS `task.reordered` event withheld by the kanban handler
    // while this band's request was still in flight.
    storeState.kanbanMulti.withheldReorderByBandKey[`${STEP_ID}:admitted`] = {
      revision: 2,
      tasks: [
        { id: "a", position: 0 },
        { id: "b", position: 1 },
        { id: "c", position: 2 },
      ],
    };

    await act(async () => {
      resolve({
        workflow_step_id: STEP_ID,
        revision: 1,
        tasks: [
          { id: "c", position: 0 },
          { id: "a", position: 1 },
          { id: "b", position: 2 },
        ],
      });
      await pending;
    });

    const tasks = storeState.kanbanMulti.snapshots[WORKFLOW_ID].tasks;
    expect(tasks.find((t) => t.id === "a")?.position).toBe(0);
    expect(tasks.find((t) => t.id === "b")?.position).toBe(1);
    expect(tasks.find((t) => t.id === "c")?.position).toBe(2);
    expect(storeState.kanbanMulti.orderRevisionByStepId[STEP_ID]).toBe(2);
    expect(storeState.kanbanMulti.withheldReorderByBandKey[`${STEP_ID}:admitted`]).toBeUndefined();
  });

  it("lets the response win a same-revision tie against a withheld order", async () => {
    const { promise, resolve } = deferred<{
      workflow_step_id: string;
      revision: number;
      tasks: Array<{ id: string; position: number }>;
    }>();
    reorderStepTasks.mockReturnValue(promise);
    const { result } = renderHook(() => useStepReorder());

    let pending!: Promise<void>;
    act(() => {
      pending = result.current.reorderBand({
        workflowId: WORKFLOW_ID,
        stepId: STEP_ID,
        band: "admitted",
        draggedId: "c",
        visibleOrderAfterMove: ["c", "a", "b"],
      });
    });

    storeState.kanbanMulti.withheldReorderByBandKey[`${STEP_ID}:admitted`] = {
      revision: 1,
      tasks: [{ id: "a", position: 77 }],
    };

    await act(async () => {
      resolve({
        workflow_step_id: STEP_ID,
        revision: 1,
        tasks: [
          { id: "c", position: 0 },
          { id: "a", position: 1 },
          { id: "b", position: 2 },
        ],
      });
      await pending;
    });

    const tasks = storeState.kanbanMulti.snapshots[WORKFLOW_ID].tasks;
    expect(tasks.find((t) => t.id === "a")?.position).toBe(1);
  });
});

describe("useStepReorder — in-flight reconciliation (AC.27), conflict and sibling-band paths", () => {
  it("reconciles against a withheld higher-revision order on a step_changed conflict too", async () => {
    storeState.kanbanMulti.withheldReorderByBandKey[`${STEP_ID}:admitted`] = {
      revision: 9,
      tasks: [
        { id: "a", position: 10 },
        { id: "b", position: 11 },
        { id: "c", position: 12 },
      ],
    };
    reorderStepTasks.mockRejectedValue(
      new ApiError("conflict", 409, {
        code: "step_changed",
        workflow_step_id: STEP_ID,
        revision: 5,
        tasks: [
          { id: "a", position: 0 },
          { id: "c", position: 1 },
          { id: "b", position: 2 },
        ],
      }),
    );
    const { result } = renderHook(() => useStepReorder());

    await act(async () => {
      await result.current.reorderBand({
        workflowId: WORKFLOW_ID,
        stepId: STEP_ID,
        band: "admitted",
        draggedId: "b",
        visibleOrderAfterMove: ["b", "a", "c"],
      });
    });

    const tasks = storeState.kanbanMulti.snapshots[WORKFLOW_ID].tasks;
    expect(tasks.find((t) => t.id === "a")?.position).toBe(10);
    expect(tasks.find((t) => t.id === "b")?.position).toBe(11);
    expect(tasks.find((t) => t.id === "c")?.position).toBe(12);
    expect(storeState.kanbanMulti.orderRevisionByStepId[STEP_ID]).toBe(9);
    expect(storeState.kanbanMulti.withheldReorderByBandKey[`${STEP_ID}:admitted`]).toBeUndefined();
  });

  it("does not let a stale whole-step response clobber the sibling band's already-fresher positions", async () => {
    resetStore([
      admittedTask("a", 0),
      admittedTask("b", 1),
      queuedTask("q1", 2),
      queuedTask("q2", 3),
    ]);
    const { promise, resolve } = deferred<{
      workflow_step_id: string;
      revision: number;
      tasks: Array<{ id: string; position: number }>;
    }>();
    reorderStepTasks.mockReturnValue(promise);
    const { result } = renderHook(() => useStepReorder());

    let pending!: Promise<void>;
    act(() => {
      pending = result.current.reorderBand({
        workflowId: WORKFLOW_ID,
        stepId: STEP_ID,
        band: "admitted",
        draggedId: "b",
        visibleOrderAfterMove: ["b", "a"],
      });
    });

    // A sibling-band WS event is applied locally while this band's request
    // is still in flight (kanban.ts applies it immediately since only the
    // admitted band is pending).
    storeState.kanbanMulti.snapshots[WORKFLOW_ID] = {
      tasks: storeState.kanbanMulti.snapshots[WORKFLOW_ID].tasks.map((t) =>
        t.id === "q1" ? { ...t, position: 9 } : t,
      ),
    };

    await act(async () => {
      resolve({
        workflow_step_id: STEP_ID,
        revision: 1,
        tasks: [
          { id: "b", position: 0 },
          { id: "a", position: 1 },
          // Stale queued-band snapshot from before the fresher WS update above.
          { id: "q1", position: 2 },
          { id: "q2", position: 3 },
        ],
      });
      await pending;
    });

    const tasks = storeState.kanbanMulti.snapshots[WORKFLOW_ID].tasks;
    expect(tasks.find((t) => t.id === "q1")?.position).toBe(9);
  });
});

describe("useStepReorder — in-flight reconciliation (AC.27), plain-failure sibling-band paths", () => {
  it("does not let a plain request failure roll back the sibling band's already-fresher positions", async () => {
    resetStore([
      admittedTask("a", 0),
      admittedTask("b", 1),
      queuedTask("q1", 2),
      queuedTask("q2", 3),
    ]);
    const { promise, reject } = deferred<{
      workflow_step_id: string;
      revision: number;
      tasks: Array<{ id: string; position: number }>;
    }>();
    reorderStepTasks.mockReturnValue(promise);
    const { result } = renderHook(() => useStepReorder());

    let pending!: Promise<void>;
    act(() => {
      pending = result.current.reorderBand({
        workflowId: WORKFLOW_ID,
        stepId: STEP_ID,
        band: "admitted",
        draggedId: "b",
        visibleOrderAfterMove: ["b", "a"],
      });
    });

    // A sibling-band WS event is applied locally while this band's request
    // is still in flight, exactly as in the success-path test above.
    storeState.kanbanMulti.snapshots[WORKFLOW_ID] = {
      tasks: storeState.kanbanMulti.snapshots[WORKFLOW_ID].tasks.map((t) =>
        t.id === "q1" ? { ...t, position: 9 } : t,
      ),
    };

    await act(async () => {
      reject(new Error("network error"));
      await pending;
    });

    const tasks = storeState.kanbanMulti.snapshots[WORKFLOW_ID].tasks;
    // The sibling (queued) band's fresher WS-applied position must survive a
    // failure of the unrelated admitted-band request.
    expect(tasks.find((t) => t.id === "q1")?.position).toBe(9);
    // This band reverts to its own pre-drag positions.
    expect(tasks.find((t) => t.id === "a")?.position).toBe(0);
    expect(tasks.find((t) => t.id === "b")?.position).toBe(1);
  });

  it("applies a withheld higher-revision order on a plain request failure without rolling back the sibling band", async () => {
    resetStore([
      admittedTask("a", 0),
      admittedTask("b", 1),
      queuedTask("q1", 2),
      queuedTask("q2", 3),
    ]);
    const { promise, reject } = deferred<{
      workflow_step_id: string;
      revision: number;
      tasks: Array<{ id: string; position: number }>;
    }>();
    reorderStepTasks.mockReturnValue(promise);
    const { result } = renderHook(() => useStepReorder());

    let pending!: Promise<void>;
    act(() => {
      pending = result.current.reorderBand({
        workflowId: WORKFLOW_ID,
        stepId: STEP_ID,
        band: "admitted",
        draggedId: "b",
        visibleOrderAfterMove: ["b", "a"],
      });
    });

    // A whole-step task.reordered event arrives while this band's request is
    // in flight: the sibling (queued) band's positions are applied live, and
    // this band's own positions are withheld under AC.27.
    storeState.kanbanMulti.snapshots[WORKFLOW_ID] = {
      tasks: storeState.kanbanMulti.snapshots[WORKFLOW_ID].tasks.map((t) =>
        t.id === "q1" ? { ...t, position: 9 } : t,
      ),
    };
    storeState.kanbanMulti.withheldReorderByBandKey[`${STEP_ID}:admitted`] = {
      revision: 3,
      tasks: [
        { id: "a", position: 20 },
        { id: "b", position: 21 },
      ],
    };

    await act(async () => {
      reject(new Error("network error"));
      await pending;
    });

    const tasks = storeState.kanbanMulti.snapshots[WORKFLOW_ID].tasks;
    expect(tasks.find((t) => t.id === "q1")?.position).toBe(9);
    expect(tasks.find((t) => t.id === "a")?.position).toBe(20);
    expect(tasks.find((t) => t.id === "b")?.position).toBe(21);
    expect(storeState.kanbanMulti.orderRevisionByStepId[STEP_ID]).toBe(3);
    expect(storeState.kanbanMulti.withheldReorderByBandKey[`${STEP_ID}:admitted`]).toBeUndefined();
  });
});
