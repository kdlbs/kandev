import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

const archiveTaskMock = vi.fn();
const deleteTaskMock = vi.fn();
const removeTaskFromBoardMock = vi.fn();
const runTaskRemovalMock = vi.fn();
const getStateMock = vi.fn();
const storeMock = { getState: getStateMock };
const replaceTaskUrlMock = vi.fn();
const setActiveSessionMock = vi.fn();
const setActiveTaskMock = vi.fn();
const toastMock = vi.fn();
let storeState: {
  tasks: { activeTaskId: string | null; activeSessionId: string | null };
  setActiveSession: (...args: unknown[]) => void;
  setActiveTask: (...args: unknown[]) => void;
};

vi.mock("@/lib/api", () => ({
  archiveTask: (...args: unknown[]) => archiveTaskMock(...args),
  deleteTask: (...args: unknown[]) => deleteTaskMock(...args),
  moveTask: vi.fn(),
  updateTask: vi.fn(),
}));

vi.mock("@/lib/links", () => ({
  replaceTaskUrl: (...args: unknown[]) => replaceTaskUrlMock(...args),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => storeMock,
}));

vi.mock("@/hooks/use-task-removal", () => ({
  useTaskRemovalSuccessNotifier: () => vi.fn(),
  useTaskRemoval: () => ({
    removeTaskFromBoard: (...args: unknown[]) => removeTaskFromBoardMock(...args),
    runTaskRemoval: (...args: unknown[]) => runTaskRemovalMock(...args),
  }),
}));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: toastMock }),
}));

import {
  useArchiveAndSwitchTask,
  useDeleteAndSwitchTask,
  useTaskActions,
} from "./use-task-actions";

beforeEach(() => {
  vi.clearAllMocks();
  storeState = {
    tasks: { activeTaskId: "task-A", activeSessionId: "sess-A" },
    setActiveSession: (...args: unknown[]) => setActiveSessionMock(...args),
    setActiveTask: (...args: unknown[]) => setActiveTaskMock(...args),
  };
  getStateMock.mockReturnValue(storeState);
  removeTaskFromBoardMock.mockResolvedValue({ switchedTaskId: "task-B" });
  runTaskRemovalMock.mockImplementation(
    async (_action: string, request: { mutate: () => Promise<void> }) => {
      await request.mutate();
      return {
        skipped: false,
        operationToken: "removal-1",
        switchedTaskId: "task-B",
        succeededTaskIds: ["task-A"],
        failedTaskIds: [],
      };
    },
  );
});

describe("useArchiveAndSwitchTask", () => {
  it("delegates archive mutation and navigation to the shared coordinator", async () => {
    const { result } = renderHook(() => useArchiveAndSwitchTask());

    await act(() => result.current("task-A"));

    expect(runTaskRemovalMock).toHaveBeenCalledWith(
      "archive",
      { taskId: "task-A", mutate: expect.any(Function) },
      { cascade: undefined },
    );
    expect(archiveTaskMock).toHaveBeenCalledWith("task-A", undefined);
  });

  it("propagates coordinator failures without a second rollback", async () => {
    const error = new Error("network error");
    runTaskRemovalMock.mockRejectedValueOnce(error);
    const { result } = renderHook(() => useArchiveAndSwitchTask());

    await expect(result.current("task-A")).rejects.toThrow("network error");

    expect(setActiveSessionMock).not.toHaveBeenCalled();
    expect(replaceTaskUrlMock).not.toHaveBeenCalled();
    expect(archiveTaskMock).not.toHaveBeenCalled();
  });

  it("passes the cascade choice to the shared coordinator", async () => {
    archiveTaskMock.mockResolvedValueOnce(undefined);
    const { result } = renderHook(() => useArchiveAndSwitchTask());

    await result.current("task-A", { cascade: true });

    expect(runTaskRemovalMock).toHaveBeenCalledWith(
      "archive",
      { taskId: "task-A", mutate: expect.any(Function) },
      { cascade: true },
    );
  });

  it("does not restore a task when the coordinator rejects before mutation", async () => {
    const error = new Error("archive failed");
    runTaskRemovalMock.mockRejectedValueOnce(error);
    const { result } = renderHook(() => useArchiveAndSwitchTask());

    await expect(result.current("task-A", { cascade: true })).rejects.toThrow("archive failed");

    expect(setActiveSessionMock).not.toHaveBeenCalled();
    expect(setActiveTaskMock).not.toHaveBeenCalled();
    expect(replaceTaskUrlMock).not.toHaveBeenCalled();
  });
});

describe("useDeleteAndSwitchTask", () => {
  it("delegates delete mutation and navigation to the shared coordinator", async () => {
    const { result } = renderHook(() => useDeleteAndSwitchTask());

    await act(() => result.current("task-A"));

    expect(runTaskRemovalMock).toHaveBeenCalledWith(
      "delete",
      { taskId: "task-A", mutate: expect.any(Function) },
      { cascade: undefined },
    );
    expect(deleteTaskMock).toHaveBeenCalledWith("task-A", undefined);
  });

  it("propagates coordinator failures without a second rollback", async () => {
    const error = new Error("delete failed");
    runTaskRemovalMock.mockRejectedValueOnce(error);
    const { result } = renderHook(() => useDeleteAndSwitchTask());

    await expect(result.current("task-A")).rejects.toThrow("delete failed");

    expect(setActiveSessionMock).not.toHaveBeenCalled();
    expect(setActiveTaskMock).not.toHaveBeenCalled();
    expect(replaceTaskUrlMock).not.toHaveBeenCalled();
    expect(deleteTaskMock).not.toHaveBeenCalled();
  });
});

describe("useTaskActions", () => {
  it("shows localized retry guidance for a dirty-worktree delete conflict", async () => {
    const { ApiError } = await import("@/lib/api/client");
    deleteTaskMock.mockRejectedValueOnce(
      new ApiError("task worktree contains local changes", 409, {
        error_code: "task_delete_dirty_worktree",
      }),
    );
    const { result } = renderHook(() => useTaskActions());

    await expect(result.current.deleteTaskById("task-1")).rejects.toMatchObject({ status: 409 });
    expect(toastMock).toHaveBeenCalledWith({
      title: "Task deletion needs your confirmation",
      description:
        "The task stays visible. Review the local changes, then select discard and try again.",
      variant: "error",
    });
  });
});
