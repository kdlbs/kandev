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
  it("does not clear prior feedback when the move cannot be attempted", async () => {
    // Missing snapshot/task rows return without touching the server, so wiping
    // a still-accurate banner on the way past them explains nothing.
    const store = buildStore({ tasks: [] });
    const onMoveStart = vi.fn();

    const { result } = renderHook(() => useMoveToStep(store as never, onMoveStart, vi.fn()));

    await act(async () => {
      await result.current("missing-task", "wf-1", "step-b");
    });
    await act(async () => {
      await result.current("task-1", "wf-missing", "step-b");
    });

    expect(onMoveStart).not.toHaveBeenCalled();
    expect(moveTaskByIdMock).not.toHaveBeenCalled();
  });

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
    // The same staleness signal that skips the rollback must also suppress the
    // banner: the newer move landed, so reporting would describe a move the
    // user abandoned while the task sits where they asked for it.
    expect(onMoveError).not.toHaveBeenCalled();
    consoleErrorSpy.mockRestore();
  });
});

describe("useMoveToStep concurrency across tasks", () => {
  it("still rolls back a rejected move after a different task moved", async () => {
    // The sidebar shares one hook across every row, so a move of task-2 must not
    // make task-1's pending move look superseded and strand it in a step the
    // backend refused.
    const store = buildStore({
      tasks: [
        { id: "task-1", workflowStepId: "step-a", position: 0 },
        { id: "task-2", workflowStepId: "step-a", position: 1 },
      ],
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

    // An unrelated task moves and succeeds while task-1 is still in flight.
    moveTaskByIdMock.mockResolvedValueOnce(undefined);
    await act(async () => {
      await result.current("task-2", "wf-1", "step-c");
    });

    const error = new Error("task has an active session (RUNNING)");
    rejectFirstMove(error);
    await act(async () => {
      await firstMove;
    });

    const tasks = store.getSnapshots()["wf-1"].tasks;
    expect(tasks.find((t) => t.id === "task-1")).toMatchObject({ workflowStepId: "step-a" });
    expect(tasks.find((t) => t.id === "task-2")).toMatchObject({ workflowStepId: "step-c" });
    expect(onMoveError).toHaveBeenCalledWith(error);
    consoleErrorSpy.mockRestore();
  });
});

describe("useMoveToStep concurrency for one task", () => {
  it("does not roll back when a same-step retry is still in flight", async () => {
    // Two moves to the same step compute identical optimistic values, so the
    // snapshot cannot distinguish them; only the generation can.
    const store = buildStore({
      tasks: [{ id: "task-1", workflowStepId: "step-a", position: 0 }],
    });
    const onMoveError = vi.fn();
    let rejectFirstMove!: (error: unknown) => void;
    moveTaskByIdMock.mockReturnValueOnce(new Promise((_res, rej) => (rejectFirstMove = rej)));
    let resolveSecondMove!: () => void;
    moveTaskByIdMock.mockReturnValueOnce(new Promise<void>((res) => (resolveSecondMove = res)));
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});

    const { result } = renderHook(() => useMoveToStep(store as never, vi.fn(), onMoveError));

    let firstMove!: Promise<void>;
    let secondMove!: Promise<void>;
    act(() => {
      firstMove = result.current("task-1", "wf-1", "step-b");
      secondMove = result.current("task-1", "wf-1", "step-b");
    });

    rejectFirstMove(new Error("stale rejection"));
    await act(async () => {
      await firstMove;
    });

    // The second move is still pending, so its optimistic state must stand.
    expect(store.getSnapshots()["wf-1"].tasks[0]).toMatchObject({ workflowStepId: "step-b" });
    expect(onMoveError).not.toHaveBeenCalled();

    resolveSecondMove();
    await act(async () => {
      await secondMove;
    });
    consoleErrorSpy.mockRestore();
  });
});

describe("useMoveToStep rollback after overlapping failures", () => {
  it("rolls back to the committed position when overlapping moves both fail", async () => {
    const store = buildStore({
      tasks: [{ id: "task-1", workflowStepId: "step-a", position: 0 }],
    });
    const onMoveError = vi.fn();
    let rejectFirstMove!: (error: unknown) => void;
    let rejectSecondMove!: (error: unknown) => void;
    moveTaskByIdMock
      .mockReturnValueOnce(new Promise((_resolve, reject) => (rejectFirstMove = reject)))
      .mockReturnValueOnce(new Promise((_resolve, reject) => (rejectSecondMove = reject)));
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});

    const { result } = renderHook(() => useMoveToStep(store as never, vi.fn(), onMoveError));

    let firstMove!: Promise<void>;
    let secondMove!: Promise<void>;
    act(() => {
      firstMove = result.current("task-1", "wf-1", "step-b");
      secondMove = result.current("task-1", "wf-1", "step-c");
    });

    rejectFirstMove(new Error("first move rejected"));
    await act(async () => {
      await firstMove;
    });
    expect(store.getSnapshots()["wf-1"].tasks[0]).toMatchObject({
      workflowStepId: "step-c",
    });

    const secondError = new Error("second move rejected");
    rejectSecondMove(secondError);
    await act(async () => {
      await secondMove;
    });

    expect(store.getSnapshots()["wf-1"].tasks[0]).toMatchObject({
      workflowStepId: "step-a",
      position: 0,
    });
    expect(onMoveError).toHaveBeenCalledWith(secondError);
    consoleErrorSpy.mockRestore();
  });
});

describe("useMoveToStep rollback after an earlier success", () => {
  it("keeps an earlier successful position when the latest move fails first", async () => {
    const store = buildStore({
      tasks: [{ id: "task-1", workflowStepId: "step-a", position: 0 }],
    });
    const onMoveError = vi.fn();
    let resolveFirstMove!: () => void;
    let rejectSecondMove!: (error: unknown) => void;
    moveTaskByIdMock
      .mockReturnValueOnce(new Promise<void>((resolve) => (resolveFirstMove = resolve)))
      .mockReturnValueOnce(new Promise((_resolve, reject) => (rejectSecondMove = reject)));
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});

    const { result } = renderHook(() => useMoveToStep(store as never, vi.fn(), onMoveError));

    let firstMove!: Promise<void>;
    let secondMove!: Promise<void>;
    act(() => {
      firstMove = result.current("task-1", "wf-1", "step-b");
      secondMove = result.current("task-1", "wf-1", "step-c");
    });

    const secondError = new Error("second move rejected");
    rejectSecondMove(secondError);
    await act(async () => {
      await secondMove;
    });
    expect(store.getSnapshots()["wf-1"].tasks[0]).toMatchObject({
      workflowStepId: "step-c",
    });

    resolveFirstMove();
    await act(async () => {
      await firstMove;
    });

    expect(store.getSnapshots()["wf-1"].tasks[0]).toMatchObject({
      workflowStepId: "step-b",
      position: 0,
    });
    expect(onMoveError).toHaveBeenCalledWith(secondError);
    consoleErrorSpy.mockRestore();
  });
});
