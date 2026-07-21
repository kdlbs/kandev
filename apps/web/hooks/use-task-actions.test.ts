import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

const archiveTaskMock = vi.fn();
const removeTaskFromBoardMock = vi.fn();
const getStateMock = vi.fn();
const storeMock = { getState: getStateMock };
const replaceTaskUrlMock = vi.fn();
const setActiveSessionMock = vi.fn();
const setActiveTaskMock = vi.fn();
let storeState: {
  tasks: { activeTaskId: string | null; activeSessionId: string | null };
  setActiveSession: (...args: unknown[]) => void;
  setActiveTask: (...args: unknown[]) => void;
};

vi.mock("@/lib/api", () => ({
  archiveTask: (...args: unknown[]) => archiveTaskMock(...args),
  deleteTask: vi.fn(),
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
  useTaskRemoval: () => ({
    removeTaskFromBoard: (...args: unknown[]) => removeTaskFromBoardMock(...args),
  }),
}));

import { useArchiveAndSwitchTask } from "./use-task-actions";

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

beforeEach(() => {
  vi.clearAllMocks();
  storeState = {
    tasks: { activeTaskId: "task-A", activeSessionId: "sess-A" },
    setActiveSession: (...args: unknown[]) => setActiveSessionMock(...args),
    setActiveTask: (...args: unknown[]) => setActiveTaskMock(...args),
  };
  getStateMock.mockReturnValue(storeState);
  removeTaskFromBoardMock.mockResolvedValue({ switchedTaskId: "task-B" });
});

describe("useArchiveAndSwitchTask", () => {
  it("removes and switches away from the active task before archive API resolves", async () => {
    const archive = deferred<void>();
    archiveTaskMock.mockReturnValueOnce(archive.promise);
    const { result } = renderHook(() => useArchiveAndSwitchTask());

    let archiveAndSwitchPromise!: Promise<void>;
    act(() => {
      archiveAndSwitchPromise = result.current("task-A");
    });

    expect(removeTaskFromBoardMock).toHaveBeenCalledWith("task-A", {
      wasActiveTaskId: "task-A",
      wasActiveSessionId: "sess-A",
      switchOnly: true,
    });

    archive.resolve();
    await archiveAndSwitchPromise;
    expect(archiveTaskMock).toHaveBeenCalledWith("task-A", undefined);
    expect(removeTaskFromBoardMock).toHaveBeenLastCalledWith("task-A", {
      wasActiveTaskId: "task-A",
      wasActiveSessionId: "sess-A",
    });
  });

  it("restores active task when archive API rejects after switching", async () => {
    const error = new Error("network error");
    archiveTaskMock.mockRejectedValueOnce(error);
    removeTaskFromBoardMock.mockImplementationOnce(async () => {
      storeState.tasks.activeTaskId = "task-B";
      return { switchedTaskId: "task-B" };
    });
    const { result } = renderHook(() => useArchiveAndSwitchTask());

    await expect(result.current("task-A")).rejects.toThrow("network error");

    expect(removeTaskFromBoardMock).toHaveBeenCalledTimes(1);
    expect(removeTaskFromBoardMock).toHaveBeenCalledWith("task-A", {
      wasActiveTaskId: "task-A",
      wasActiveSessionId: "sess-A",
      switchOnly: true,
    });
    expect(setActiveSessionMock).toHaveBeenCalledWith("task-A", "sess-A");
    expect(replaceTaskUrlMock).toHaveBeenCalledWith("task-A");
    expect(archiveTaskMock).toHaveBeenCalledWith("task-A", undefined);
  });
});
