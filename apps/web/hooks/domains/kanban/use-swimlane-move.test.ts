import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import type { Task } from "@/components/kanban-card";
import type { KanbanState } from "@/lib/state/slices/kanban/types";

const moveTaskByIdMock = vi.fn();
const getStateMock = vi.fn();
const setWorkflowSnapshotMock = vi.fn();
const storeMock = { getState: getStateMock };

type SnapshotTask = KanbanState["tasks"][number];
type Snapshot = { steps: unknown[]; tasks: SnapshotTask[] };

let snapshots: Record<string, Snapshot>;

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => storeMock,
}));

vi.mock("@/hooks/use-task-actions", () => ({
  useTaskActions: () => ({ moveTaskById: (...args: unknown[]) => moveTaskByIdMock(...args) }),
}));

import { useSwimlaneMove } from "./use-swimlane-move";

function makeTask(overrides: Partial<SnapshotTask> & { id: string }): SnapshotTask {
  return {
    title: overrides.id,
    workflowStepId: "todo",
    position: 0,
    ...overrides,
  } as SnapshotTask;
}

function makeState() {
  return {
    get kanbanMulti() {
      return { snapshots };
    },
    setWorkflowSnapshot: (workflowId: string, next: Snapshot) => {
      snapshots = { ...snapshots, [workflowId]: next };
      setWorkflowSnapshotMock(workflowId, next);
    },
  };
}

function stateWithSnapshot(snapshot: Snapshot) {
  snapshots = { "wf-1": snapshot };
  const state = makeState();
  getStateMock.mockImplementation(() => state);
  return state;
}

const NETWORK_ERROR_MESSAGE = "network error";

function mockPendingMove() {
  const deferredReject: { reject?: (e: Error) => void } = {};
  moveTaskByIdMock.mockImplementationOnce(
    () =>
      new Promise((_resolve, reject) => {
        deferredReject.reject = reject;
      }),
  );
  return deferredReject;
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("useSwimlaneMove — failure rollback", () => {
  it("restores only the moved task's step, leaving a sibling task updated mid-flight untouched", async () => {
    const moved = makeTask({ id: "task-A", workflowStepId: "todo", position: 0 });
    const sibling = makeTask({ id: "task-B", workflowStepId: "todo", position: 1 });
    stateWithSnapshot({ steps: [], tasks: [moved, sibling] });

    const deferredReject = mockPendingMove();

    const { result } = renderHook(() => useSwimlaneMove("wf-1", {}));
    let movePromise!: Promise<void>;
    act(() => {
      movePromise = result.current.moveTask(moved as unknown as Task, "doing");
    });

    // Out-of-band update lands on the sibling while move A is still in flight.
    const inFlightSnapshot = snapshots["wf-1"];
    const outOfBandSnapshot = {
      ...inFlightSnapshot,
      tasks: inFlightSnapshot.tasks.map((t) =>
        t.id === "task-B" ? { ...t, workflowStepId: "done", position: 0 } : t,
      ),
    };
    snapshots = { "wf-1": outOfBandSnapshot };
    getStateMock.mockImplementation(makeState);

    deferredReject.reject?.(new Error(NETWORK_ERROR_MESSAGE));
    await act(async () => {
      await movePromise;
    });

    const siblingAfter = snapshots["wf-1"].tasks.find((t) => t.id === "task-B");
    expect(siblingAfter?.workflowStepId).toBe("done");
    const movedAfter = snapshots["wf-1"].tasks.find((t) => t.id === "task-A");
    expect(movedAfter?.workflowStepId).toBe("todo");
  });

  it("skips the rollback when an out-of-band update already superseded the moved task's own entry", async () => {
    const moved = makeTask({ id: "task-A", workflowStepId: "todo", position: 0 });
    stateWithSnapshot({ steps: [], tasks: [moved] });

    const deferredReject = mockPendingMove();

    const { result } = renderHook(() => useSwimlaneMove("wf-1", {}));
    let movePromise!: Promise<void>;
    act(() => {
      movePromise = result.current.moveTask(moved as unknown as Task, "doing");
    });

    // Before the move settles, an out-of-band update moves the same task
    // again (a newer value than the optimistic write this move made).
    const inFlightSnapshot = snapshots["wf-1"];
    const supersededSnapshot = {
      ...inFlightSnapshot,
      tasks: inFlightSnapshot.tasks.map((t) =>
        t.id === "task-A" ? { ...t, workflowStepId: "review", position: 0 } : t,
      ),
    };
    snapshots = { "wf-1": supersededSnapshot };
    getStateMock.mockImplementation(makeState);

    deferredReject.reject?.(new Error(NETWORK_ERROR_MESSAGE));
    await act(async () => {
      await movePromise;
    });

    // The failed move's rollback must not clobber the newer out-of-band step.
    expect(snapshots["wf-1"].tasks.find((t) => t.id === "task-A")?.workflowStepId).toBe("review");
  });

  it("skips the rollback when a concurrent move to the same target step and position already succeeded", async () => {
    // Two independent moves of the same task to the same, currently-empty
    // target step both compute position 0 (AC-UI-PIPELINE-ROW-005.3): this
    // move's own optimistic write and a concurrent one that actually landed
    // are value-identical, so the rollback predicate must not treat that
    // coincidence as proof the store still holds *this* move's write.
    const moved = makeTask({ id: "task-A", workflowStepId: "todo", position: 0, updatedAt: "t0" });
    stateWithSnapshot({ steps: [], tasks: [moved] });

    const deferredReject = mockPendingMove();

    const { result } = renderHook(() => useSwimlaneMove("wf-1", {}));
    let movePromise!: Promise<void>;
    act(() => {
      movePromise = result.current.moveTask(moved as unknown as Task, "doing");
    });

    // A concurrent, genuinely successful move lands on the exact same
    // (step, position) this request's own optimistic write predicted, but
    // with a fresh server-assigned updatedAt.
    const inFlightSnapshot = snapshots["wf-1"];
    const concurrentSuccessSnapshot = {
      ...inFlightSnapshot,
      tasks: inFlightSnapshot.tasks.map((t) =>
        t.id === "task-A" ? { ...t, workflowStepId: "doing", position: 0, updatedAt: "t1" } : t,
      ),
    };
    snapshots = { "wf-1": concurrentSuccessSnapshot };
    getStateMock.mockImplementation(makeState);

    deferredReject.reject?.(new Error(NETWORK_ERROR_MESSAGE));
    await act(async () => {
      await movePromise;
    });

    // The failed request's rollback must not clobber the other actor's real,
    // successful move back to the stale pre-move step.
    expect(snapshots["wf-1"].tasks.find((t) => t.id === "task-A")?.workflowStepId).toBe("doing");
    expect(snapshots["wf-1"].tasks.find((t) => t.id === "task-A")?.updatedAt).toBe("t1");
  });

  it("restores the moved task to its pre-move step on a plain failure", async () => {
    const moved = makeTask({ id: "task-A", workflowStepId: "todo", position: 0 });
    stateWithSnapshot({ steps: [], tasks: [moved] });
    moveTaskByIdMock.mockRejectedValueOnce(new Error(NETWORK_ERROR_MESSAGE));

    const { result } = renderHook(() => useSwimlaneMove("wf-1", {}));
    await act(async () => {
      await result.current.moveTask(moved as unknown as Task, "doing");
    });

    expect(snapshots["wf-1"].tasks.find((t) => t.id === "task-A")?.workflowStepId).toBe("todo");
  });
});
