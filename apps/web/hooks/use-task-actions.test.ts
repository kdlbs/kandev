import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

const archiveTaskMock = vi.fn();
const removeTaskFromBoardMock = vi.fn();
const getStateMock = vi.fn();
const storeMock = { getState: getStateMock };

vi.mock("@/lib/api", () => ({
  archiveTask: (...args: unknown[]) => archiveTaskMock(...args),
  deleteTask: vi.fn(),
  moveTask: vi.fn(),
  updateTask: vi.fn(),
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
  getStateMock.mockReturnValue({
    tasks: { activeTaskId: "task-A", activeSessionId: "sess-A" },
  });
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
      removeFromBoard: false,
    });

    archive.resolve();
    await archiveAndSwitchPromise;
    expect(archiveTaskMock).toHaveBeenCalledWith("task-A", undefined);
    expect(removeTaskFromBoardMock).toHaveBeenLastCalledWith("task-A", {
      wasActiveTaskId: "task-A",
      wasActiveSessionId: "sess-A",
    });
  });
});
