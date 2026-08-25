import { describe, expect, it, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

const moveTaskByIdMock = vi.fn();

vi.mock("@/hooks/use-task-actions", () => ({
  useTaskActions: () => ({ moveTaskById: (...args: unknown[]) => moveTaskByIdMock(...args) }),
}));

import { useMoveToStep } from "./task-session-sidebar-move";

type Task = { id: string; workflowStepId: string; position: number };
type Snapshot = { tasks: Task[] };

function buildStore(initialSnapshot: Snapshot) {
  let snapshots: Record<string, Snapshot> = { "wf-1": initialSnapshot };
  return {
    getState: () => ({
      kanbanMulti: { snapshots },
      setWorkflowSnapshot: (workflowId: string, data: Snapshot) => {
        snapshots = { ...snapshots, [workflowId]: data };
      },
    }),
    getSnapshots: () => snapshots,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("useMoveToStep", () => {
  it("calls onMoveStart, then optimistically moves the task before the request resolves", async () => {
    const store = buildStore({
      tasks: [{ id: "task-1", workflowStepId: "step-a", position: 0 }],
    });
    const onMoveStart = vi.fn();
    let resolveMove!: () => void;
    moveTaskByIdMock.mockReturnValueOnce(new Promise<void>((res) => (resolveMove = res)));

    const { result } = renderHook(() => useMoveToStep(store as never, onMoveStart, vi.fn()));

    let movePromise!: Promise<void>;
    act(() => {
      movePromise = result.current("task-1", "wf-1", "step-b");
    });

    expect(onMoveStart).toHaveBeenCalledTimes(1);
    expect(store.getSnapshots()["wf-1"].tasks[0]).toMatchObject({
      workflowStepId: "step-b",
      position: 0,
    });

    resolveMove();
    await act(async () => {
      await movePromise;
    });
  });

  it("rolls back the optimistic move and reports the error on rejection", async () => {
    const store = buildStore({
      tasks: [{ id: "task-1", workflowStepId: "step-a", position: 0 }],
    });
    const onMoveError = vi.fn();
    const error = new Error("task has an active session (RUNNING)");
    moveTaskByIdMock.mockRejectedValueOnce(error);
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});

    const { result } = renderHook(() => useMoveToStep(store as never, vi.fn(), onMoveError));

    await act(async () => {
      await result.current("task-1", "wf-1", "step-b");
    });

    expect(store.getSnapshots()["wf-1"].tasks[0]).toMatchObject({
      workflowStepId: "step-a",
      position: 0,
    });
    expect(onMoveError).toHaveBeenCalledWith(error);
    consoleErrorSpy.mockRestore();
  });

  it("does not roll back a task that moved again before the rejection arrived", async () => {
    const store = buildStore({
      tasks: [{ id: "task-1", workflowStepId: "step-a", position: 0 }],
    });
    const onMoveError = vi.fn();
    let rejectFirstMove!: (error: unknown) => void;
    moveTaskByIdMock.mockReturnValueOnce(new Promise((_res, rej) => (rejectFirstMove = rej)));
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});

    const { result } = renderHook(() => useMoveToStep(store as never, vi.fn(), onMoveError));

    let firstMove!: Promise<void>;
    act(() => {
      firstMove = result.current("task-1", "wf-1", "step-b");
    });

    // A later move (e.g. the user retried to a different step) supersedes the pending one.
    moveTaskByIdMock.mockResolvedValueOnce(undefined);
    await act(async () => {
      await result.current("task-1", "wf-1", "step-c");
    });

    rejectFirstMove(new Error("stale rejection"));
    await act(async () => {
      await firstMove;
    });

    expect(store.getSnapshots()["wf-1"].tasks[0]).toMatchObject({
      workflowStepId: "step-c",
    });
    consoleErrorSpy.mockRestore();
  });
});
