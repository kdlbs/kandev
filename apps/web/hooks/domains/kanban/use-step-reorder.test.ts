import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";

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

let storeState: {
  kanbanMulti: {
    snapshots: Record<string, { tasks: FakeTask[] }>;
    pendingReorderBandKeys: Record<string, true>;
    orderRevisionByStepId: Record<string, number>;
  };
  setWorkflowSnapshot: (wfId: string, snapshot: { tasks: FakeTask[] }) => void;
  setBandReorderPending: (stepId: string, band: string, pending: boolean) => void;
  setStepOrderRevision: (stepId: string, revision: number) => void;
};

const WORKFLOW_ID = "wf1";
const STEP_ID = "step-1";

function admittedTask(id: string, position: number): FakeTask {
  return { id, workflowStepId: STEP_ID, position, wipAdmitted: true };
}

function resetStore(tasks: FakeTask[]) {
  storeState = {
    kanbanMulti: {
      snapshots: { [WORKFLOW_ID]: { tasks } },
      pendingReorderBandKeys: {},
      orderRevisionByStepId: {},
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

describe("useStepReorder", () => {
  it("submits the band's full reordered membership and applies the returned positions", async () => {
    reorderStepTasks.mockResolvedValue({
      workflow_step_id: STEP_ID,
      revision: 1,
      tasks: [
        { id: "b", position: 0 },
        { id: "a", position: 1 },
        { id: "c", position: 2 },
      ],
    });
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

    expect(reorderStepTasks).toHaveBeenCalledWith(STEP_ID, {
      band: "admitted",
      ordered_task_ids: ["b", "a", "c"],
    });
    const tasks = storeState.kanbanMulti.snapshots[WORKFLOW_ID].tasks;
    expect(tasks.find((t) => t.id === "b")?.position).toBe(0);
    expect(tasks.find((t) => t.id === "a")?.position).toBe(1);
    expect(tasks.find((t) => t.id === "c")?.position).toBe(2);
    expect(storeState.kanbanMulti.orderRevisionByStepId[STEP_ID]).toBe(1);
  });

  it("applies the optimistic order and marks the band pending before the request resolves", async () => {
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

    // Optimistic order applied immediately (AC.8), before the request settles.
    const tasks = storeState.kanbanMulti.snapshots[WORKFLOW_ID].tasks;
    expect(tasks.find((t) => t.id === "c")?.position).toBe(0);
    expect(storeState.kanbanMulti.pendingReorderBandKeys[`${STEP_ID}:admitted`]).toBe(true);

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

    expect(storeState.kanbanMulti.pendingReorderBandKeys[`${STEP_ID}:admitted`]).toBeUndefined();
  });

  it("issues no request when the drop reproduces the order already displayed (AC.10)", async () => {
    const { result } = renderHook(() => useStepReorder());

    await act(async () => {
      await result.current.reorderBand({
        workflowId: WORKFLOW_ID,
        stepId: STEP_ID,
        band: "admitted",
        draggedId: "a",
        visibleOrderAfterMove: ["a", "b", "c"],
      });
    });

    expect(reorderStepTasks).not.toHaveBeenCalled();
  });
});

describe("useStepReorder — board filter membership", () => {
  it("keeps a board-filtered-out task in the band without reordering it (AC.34)", async () => {
    // Full membership includes "h", a task the current board filter hides;
    // the caller (the board) only ever sees/submits a, b, c.
    resetStore([
      admittedTask("a", 0),
      admittedTask("b", 1),
      admittedTask("h", 2),
      admittedTask("c", 3),
    ]);
    reorderStepTasks.mockResolvedValue({
      workflow_step_id: STEP_ID,
      revision: 1,
      tasks: [
        { id: "c", position: 0 },
        { id: "a", position: 1 },
        { id: "b", position: 2 },
        { id: "h", position: 3 },
      ],
    });
    const { result } = renderHook(() => useStepReorder());

    await act(async () => {
      await result.current.reorderBand({
        workflowId: WORKFLOW_ID,
        stepId: STEP_ID,
        band: "admitted",
        draggedId: "c",
        visibleOrderAfterMove: ["c", "a", "b"],
      });
    });

    // The hidden task's full-membership order is preserved (still last,
    // behind a and b) — only the dragged task moved.
    expect(reorderStepTasks).toHaveBeenCalledWith(STEP_ID, {
      band: "admitted",
      ordered_task_ids: ["c", "a", "b", "h"],
    });
    const tasks = storeState.kanbanMulti.snapshots[WORKFLOW_ID].tasks;
    expect(tasks.find((t) => t.id === "c")?.position).toBe(0);
    expect(tasks.find((t) => t.id === "a")?.position).toBe(1);
    expect(tasks.find((t) => t.id === "b")?.position).toBe(2);
    expect(tasks.find((t) => t.id === "h")?.position).toBe(3);
  });
});

describe("useStepReorder — failure handling", () => {
  it("reconciles silently to the authoritative order on a step_changed conflict, without a toast", async () => {
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

    expect(mockToast).not.toHaveBeenCalled();
    const tasks = storeState.kanbanMulti.snapshots[WORKFLOW_ID].tasks;
    expect(tasks.find((t) => t.id === "a")?.position).toBe(0);
    expect(tasks.find((t) => t.id === "c")?.position).toBe(1);
    expect(tasks.find((t) => t.id === "b")?.position).toBe(2);
    expect(storeState.kanbanMulti.orderRevisionByStepId[STEP_ID]).toBe(5);
    expect(storeState.kanbanMulti.pendingReorderBandKeys[`${STEP_ID}:admitted`]).toBeUndefined();
  });

  it("restores the original order and shows a localized error toast on any other failure (AC.20)", async () => {
    reorderStepTasks.mockRejectedValue(
      new ApiError("bad request", 400, { code: "invalid_reorder" }),
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
    expect(tasks.find((t) => t.id === "a")?.position).toBe(0);
    expect(tasks.find((t) => t.id === "b")?.position).toBe(1);
    expect(tasks.find((t) => t.id === "c")?.position).toBe(2);
    expect(mockToast).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Failed to reorder tasks", variant: "error" }),
    );
    expect(storeState.kanbanMulti.pendingReorderBandKeys[`${STEP_ID}:admitted`]).toBeUndefined();
  });

  it("refuses to start a second request for a band that already has one in flight (AC.27)", async () => {
    const { promise } = deferred<{
      workflow_step_id: string;
      revision: number;
      tasks: Array<{ id: string; position: number }>;
    }>();
    reorderStepTasks.mockReturnValue(promise);
    const { result } = renderHook(() => useStepReorder());

    act(() => {
      void result.current.reorderBand({
        workflowId: WORKFLOW_ID,
        stepId: STEP_ID,
        band: "admitted",
        draggedId: "c",
        visibleOrderAfterMove: ["c", "a", "b"],
      });
    });

    await act(async () => {
      await result.current.reorderBand({
        workflowId: WORKFLOW_ID,
        stepId: STEP_ID,
        band: "admitted",
        draggedId: "a",
        visibleOrderAfterMove: ["a", "c", "b"],
      });
    });

    expect(reorderStepTasks).toHaveBeenCalledTimes(1);
  });
});
