import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  archiveAndSwitch: vi.fn(),
  toast: vi.fn(),
  state: {
    kanbanMulti: { snapshots: {} as Record<string, { tasks: unknown[] }> },
    kanban: { tasks: [] as unknown[] },
  },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({ getState: () => mocks.state }),
}));
vi.mock("@/hooks/use-task-actions", () => ({
  useArchiveAndSwitchTask: () => mocks.archiveAndSwitch,
}));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));

import { useTaskArchiveConfirm } from "./use-task-archive-confirm";

beforeEach(() => {
  mocks.archiveAndSwitch.mockReset().mockResolvedValue(undefined);
  mocks.toast.mockReset();
  mocks.state.kanbanMulti.snapshots = {};
  mocks.state.kanban.tasks = [
    { id: "task-A", title: "Ship the palette", primaryExecutorType: "docker" },
  ];
});

describe("useTaskArchiveConfirm", () => {
  it("resolves the target task title and executor from the store", () => {
    const { result } = renderHook(() => useTaskArchiveConfirm("task-A"));

    expect(result.current.target).toBeNull();
    act(() => result.current.requestArchive());

    expect(result.current.target).toEqual({
      id: "task-A",
      title: "Ship the palette",
      executorType: "docker",
    });
  });

  it("falls back to generic copy when the task is not in the store", () => {
    mocks.state.kanban.tasks = [];
    const { result } = renderHook(() => useTaskArchiveConfirm("task-A"));

    act(() => result.current.requestArchive());

    expect(result.current.target?.title).toBe("this task");
    expect(result.current.target?.executorType).toBeUndefined();
  });

  it("does not open a target when there is no task", () => {
    const { result } = renderHook(() => useTaskArchiveConfirm(null));

    act(() => result.current.requestArchive());

    expect(result.current.target).toBeNull();
  });

  it("archives with the cascade choice and does not toast on success", async () => {
    const { result } = renderHook(() => useTaskArchiveConfirm("task-A"));

    await act(async () => {
      await result.current.confirmArchive({ cascade: true });
    });

    expect(mocks.archiveAndSwitch).toHaveBeenCalledWith("task-A", { cascade: true });
    expect(mocks.toast).not.toHaveBeenCalled();
    expect(result.current.isPending).toBe(false);
  });

  it("archives the requested task when the active task changes before confirmation", async () => {
    mocks.state.kanban.tasks = [
      { id: "task-A", title: "Task A", primaryExecutorType: "docker" },
      { id: "task-B", title: "Task B", primaryExecutorType: "local" },
    ];
    const { result, rerender } = renderHook(
      ({ taskId }: { taskId: string }) => useTaskArchiveConfirm(taskId),
      { initialProps: { taskId: "task-A" } },
    );

    act(() => result.current.requestArchive());
    rerender({ taskId: "task-B" });
    await act(async () => {
      await result.current.confirmArchive({ cascade: false });
    });

    expect(mocks.archiveAndSwitch).toHaveBeenCalledWith("task-A", { cascade: false });
  });

  it("toasts, logs the cause, and clears pending when archiving fails", async () => {
    const cause = new Error("boom");
    const logged = vi.spyOn(console, "error").mockImplementation(() => {});
    mocks.archiveAndSwitch.mockRejectedValueOnce(cause);
    const { result } = renderHook(() => useTaskArchiveConfirm("task-A"));

    await act(async () => {
      await result.current.confirmArchive({ cascade: false });
    });

    expect(mocks.toast).toHaveBeenCalledWith({
      description: "Failed to archive task",
      variant: "error",
    });
    // The toast is generic, so the rejection reason has to reach the console.
    expect(logged).toHaveBeenCalledWith("Failed to archive task:", cause);
    await waitFor(() => expect(result.current.isPending).toBe(false));
    logged.mockRestore();
  });

  it("never archives when there is no task", async () => {
    const { result } = renderHook(() => useTaskArchiveConfirm(null));

    await act(async () => {
      await result.current.confirmArchive({ cascade: false });
    });

    expect(mocks.archiveAndSwitch).not.toHaveBeenCalled();
  });

  it("clears the target on close", () => {
    const { result } = renderHook(() => useTaskArchiveConfirm("task-A"));

    act(() => result.current.requestArchive());
    act(() => result.current.closeConfirm());

    expect(result.current.target).toBeNull();
  });
});
