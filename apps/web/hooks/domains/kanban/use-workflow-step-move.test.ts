import { describe, it, expect, vi, afterEach } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { moveTask } from "@/lib/api";
import { usePresentationToken, useWorkflowStepMove } from "./use-workflow-step-move";

const mockAppState = vi.hoisted(() => ({
  value: {
    tasks: { activeSessionId: null as string | null },
    chatInput: { planModeBySessionId: {} as Record<string, boolean> },
    setPlanMode: vi.fn(),
    setActiveDocument: vi.fn(),
  },
}));
const mockLayoutStore = vi.hoisted(() => ({ closeDocument: vi.fn() }));
const mockContextFilesStore = vi.hoisted(() => ({ removeFile: vi.fn() }));
const mockDockviewStore = vi.hoisted(() => ({ applyBuiltInPreset: vi.fn() }));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mockAppState.value) => unknown) =>
    selector(mockAppState.value),
}));
vi.mock("@/lib/state/layout-store", () => ({
  useLayoutStore: (selector: (state: typeof mockLayoutStore) => unknown) =>
    selector(mockLayoutStore),
}));
vi.mock("@/lib/state/context-files-store", () => ({
  useContextFilesStore: (selector: (state: typeof mockContextFilesStore) => unknown) =>
    selector(mockContextFilesStore),
}));
vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (selector: (state: typeof mockDockviewStore) => unknown) =>
    selector(mockDockviewStore),
}));
vi.mock("@/lib/api", () => ({
  moveTask: vi.fn(),
}));

const TASK_ID = "task-1";
const WORKFLOW_ID = "workflow-1";

afterEach(() => {
  vi.clearAllMocks();
  mockAppState.value.tasks.activeSessionId = null;
  mockAppState.value.chatInput.planModeBySessionId = {};
});

describe("usePresentationToken", () => {
  it("changes when the key changes, and again on a return to the same key", () => {
    const { result, rerender } = renderHook(({ key }) => usePresentationToken(key), {
      initialProps: { key: "a" as string | null },
    });
    const first = result.current;

    rerender({ key: null });
    const second = result.current;
    expect(second).not.toBe(first);

    rerender({ key: "a" });
    const third = result.current;
    expect(third).not.toBe(second);
  });

  it("stays the same across a re-render with an unchanged key", () => {
    const { result, rerender } = renderHook(({ key }) => usePresentationToken(key), {
      initialProps: { key: "a" as string | null },
    });
    const first = result.current;
    rerender({ key: "a" });
    expect(result.current).toBe(first);
  });
});

describe("useWorkflowStepMove", () => {
  it("issues the move request with the task's workflow id, target step, and position 0", async () => {
    vi.mocked(moveTask).mockResolvedValue({} as Awaited<ReturnType<typeof moveTask>>);
    const { result } = renderHook(() =>
      useWorkflowStepMove({ taskId: TASK_ID, workflowId: WORKFLOW_ID, presentationToken: 0 }),
    );

    await act(async () => {
      await result.current.handleMove("step-b");
    });

    expect(moveTask).toHaveBeenCalledWith(TASK_ID, {
      workflow_id: WORKFLOW_ID,
      workflow_step_id: "step-b",
      position: 0,
    });
  });

  it("notifies onMoveStart and disables plan mode before the request resolves", async () => {
    mockAppState.value.tasks.activeSessionId = "session-1";
    mockAppState.value.chatInput.planModeBySessionId = { "session-1": true };
    vi.mocked(moveTask).mockResolvedValue({} as Awaited<ReturnType<typeof moveTask>>);
    const onMoveStart = vi.fn();
    const { result } = renderHook(() =>
      useWorkflowStepMove({
        taskId: TASK_ID,
        workflowId: WORKFLOW_ID,
        presentationToken: 0,
        onMoveStart,
      }),
    );

    await act(async () => {
      await result.current.handleMove("step-b");
    });

    expect(onMoveStart).toHaveBeenCalledTimes(1);
    expect(mockAppState.value.setPlanMode).toHaveBeenCalledWith("session-1", false);
  });

  it("reports a rejected move and re-enables the control", async () => {
    const error = new Error("network error");
    vi.mocked(moveTask).mockRejectedValueOnce(error);
    const onMoveError = vi.fn();
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const { result } = renderHook(() =>
      useWorkflowStepMove({
        taskId: TASK_ID,
        workflowId: WORKFLOW_ID,
        presentationToken: 0,
        onMoveError,
      }),
    );

    await act(async () => {
      await result.current.handleMove("step-b");
    });

    expect(onMoveError).toHaveBeenCalledWith(error);
    expect(result.current.movingToStepId).toBeNull();
    consoleErrorSpy.mockRestore();
  });

  it("retains an accepted destination until the task projection reaches it", async () => {
    vi.mocked(moveTask).mockResolvedValue({} as Awaited<ReturnType<typeof moveTask>>);
    const { result, rerender } = renderHook(
      ({ currentStepId }: { currentStepId: string }) =>
        useWorkflowStepMove({
          taskId: TASK_ID,
          workflowId: WORKFLOW_ID,
          currentStepId,
          presentationToken: 0,
        }),
      { initialProps: { currentStepId: "step-a" } },
    );

    await act(async () => {
      await result.current.handleMove("step-b");
    });

    expect(result.current.movingToStepId).toBeNull();
    expect(result.current.progressingToStepId).toBe("step-b");

    rerender({ currentStepId: "step-b" });
    expect(result.current.progressingToStepId).toBeNull();
  });

  it("returns false without calling the API when taskId or workflowId is missing", async () => {
    const { result } = renderHook(() =>
      useWorkflowStepMove({ taskId: null, workflowId: WORKFLOW_ID, presentationToken: 0 }),
    );

    const moved = await act(async () => result.current.handleMove("step-b"));

    expect(moved).toBe(false);
    expect(moveTask).not.toHaveBeenCalled();
  });
});

describe("useWorkflowStepMove deferred progress cleanup", () => {
  it("clears a deferred move when the task advances directly past its target", async () => {
    let resolveMove!: () => void;
    vi.mocked(moveTask).mockReturnValueOnce(
      new Promise<Awaited<ReturnType<typeof moveTask>>>((resolve) => {
        resolveMove = () => resolve({} as Awaited<ReturnType<typeof moveTask>>);
      }),
    );
    const { result, rerender } = renderHook(
      ({ currentStepId, taskState }: { currentStepId: string; taskState: string }) =>
        useWorkflowStepMove({
          taskId: TASK_ID,
          workflowId: WORKFLOW_ID,
          currentStepId,
          taskState,
          presentationToken: 0,
        }),
      { initialProps: { currentStepId: "step-a", taskState: "IN_PROGRESS" } },
    );

    let movePromise!: Promise<boolean>;
    act(() => {
      movePromise = result.current.handleMove("step-b");
    });
    expect(result.current.movingToStepId).toBe("step-b");

    // The authoritative task projection jumps from A to C before the held
    // response resolves. B is superseded and must lose both pending markers.
    rerender({ currentStepId: "step-c", taskState: "IN_PROGRESS" });
    expect(result.current.movingToStepId).toBeNull();
    expect(result.current.progressingToStepId).toBeNull();

    await act(async () => {
      resolveMove();
      await movePromise;
    });
    expect(result.current.movingToStepId).toBeNull();
    expect(result.current.progressingToStepId).toBeNull();
  });

  it("clears a deferred move when the task becomes terminal before its target is observed", async () => {
    let resolveMove!: () => void;
    vi.mocked(moveTask).mockReturnValueOnce(
      new Promise<Awaited<ReturnType<typeof moveTask>>>((resolve) => {
        resolveMove = () => resolve({} as Awaited<ReturnType<typeof moveTask>>);
      }),
    );
    const { result, rerender } = renderHook(
      ({ currentStepId, taskState }: { currentStepId: string; taskState: string }) =>
        useWorkflowStepMove({
          taskId: TASK_ID,
          workflowId: WORKFLOW_ID,
          currentStepId,
          taskState,
          presentationToken: 0,
        }),
      { initialProps: { currentStepId: "step-a", taskState: "IN_PROGRESS" } },
    );

    let movePromise!: Promise<boolean>;
    act(() => {
      movePromise = result.current.handleMove("step-b");
    });

    rerender({ currentStepId: "step-a", taskState: "COMPLETED" });
    expect(result.current.movingToStepId).toBeNull();
    expect(result.current.progressingToStepId).toBeNull();

    await act(async () => {
      resolveMove();
      await movePromise;
    });
    expect(result.current.progressingToStepId).toBeNull();
  });

  it("keeps the latest destination while the projection reports an older accepted target", async () => {
    vi.mocked(moveTask).mockResolvedValue({} as Awaited<ReturnType<typeof moveTask>>);
    const { result, rerender } = renderHook(
      ({ currentStepId }: { currentStepId: string }) =>
        useWorkflowStepMove({
          taskId: TASK_ID,
          workflowId: WORKFLOW_ID,
          currentStepId,
          presentationToken: 0,
        }),
      { initialProps: { currentStepId: "step-a" } },
    );

    await act(async () => {
      await result.current.handleMove("step-b");
    });

    await act(async () => {
      await result.current.handleMove("step-c");
    });

    // B is evidence for the earlier accepted request, not supersession of the
    // newer A-to-C request. Keep C visible until its own projection arrives.
    rerender({ currentStepId: "step-b" });
    expect(result.current.progressingToStepId).toBe("step-c");

    rerender({ currentStepId: "step-c" });
    expect(result.current.progressingToStepId).toBeNull();
  });
});

describe("useWorkflowStepMove presentation token handling", () => {
  it("discards a late failure once the presentation token has changed", async () => {
    let rejectMove!: (error: unknown) => void;
    vi.mocked(moveTask).mockReturnValueOnce(
      new Promise((_resolve, reject) => (rejectMove = reject)),
    );
    const onMoveError = vi.fn();
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const { result, rerender } = renderHook(
      ({ presentationToken }: { presentationToken: number }) =>
        useWorkflowStepMove({
          taskId: TASK_ID,
          workflowId: WORKFLOW_ID,
          presentationToken,
          onMoveError,
        }),
      { initialProps: { presentationToken: 0 } },
    );

    let movePromise!: Promise<boolean>;
    act(() => {
      movePromise = result.current.handleMove("step-b");
    });
    expect(result.current.movingToStepId).toBe("step-b");

    // The preview closed and reopened on the same task (or switched to a
    // different one) before the request resolved: a new presentation begins
    // with no in-flight move, even though one is still outstanding.
    rerender({ presentationToken: 1 });
    expect(result.current.movingToStepId).toBeNull();

    await act(async () => {
      rejectMove(new Error("stale failure"));
      await movePromise;
    });

    expect(onMoveError).not.toHaveBeenCalled();
    expect(result.current.movingToStepId).toBeNull();
    consoleErrorSpy.mockRestore();
  });

  it("ignores a superseded move's rejection within the same presentation", async () => {
    let rejectFirst!: (error: unknown) => void;
    vi.mocked(moveTask).mockReturnValueOnce(new Promise((_res, rej) => (rejectFirst = rej)));
    vi.mocked(moveTask).mockResolvedValueOnce({} as Awaited<ReturnType<typeof moveTask>>);
    const onMoveError = vi.fn();
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const { result } = renderHook(() =>
      useWorkflowStepMove({
        taskId: TASK_ID,
        workflowId: WORKFLOW_ID,
        presentationToken: 0,
        onMoveError,
      }),
    );

    let firstPromise!: Promise<boolean>;
    act(() => {
      firstPromise = result.current.handleMove("step-b");
    });
    await act(async () => {
      await result.current.handleMove("step-c");
    });

    await act(async () => {
      rejectFirst(new Error("stale"));
      await firstPromise;
    });

    expect(onMoveError).not.toHaveBeenCalled();
    consoleErrorSpy.mockRestore();
  });
});
