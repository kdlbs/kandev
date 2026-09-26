import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Task } from "@/lib/types/http";
import { useChangeWorkflowTask } from "./use-change-workflow-data";

const apiMocks = vi.hoisted(() => ({ fetchTask: vi.fn() }));
const TASK_ID = "task-1";
const WORKSPACE_ID = "workspace-1";

vi.mock("@/lib/api", () => ({
  fetchTask: apiMocks.fetchTask,
  fetchWorkflowSnapshot: vi.fn(),
  listAgents: vi.fn(),
  listExecutors: vi.fn(),
  listWorkflows: vi.fn(),
}));

afterEach(() => {
  cleanup();
  apiMocks.fetchTask.mockReset();
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

function task(id: string, workspaceId = WORKSPACE_ID): Task {
  return { id, workspace_id: workspaceId } as Task;
}

describe("useChangeWorkflowTask", () => {
  it("ignores an older task fetch after the task identity changes", async () => {
    const oldTask = deferred<Task>();
    apiMocks.fetchTask.mockReturnValueOnce(oldTask.promise).mockResolvedValueOnce(task("task-2"));
    const { result, rerender } = renderHook((props) => useChangeWorkflowTask(props), {
      initialProps: { open: true, taskId: TASK_ID, workspaceId: WORKSPACE_ID },
    });

    rerender({ open: true, taskId: "task-2", workspaceId: WORKSPACE_ID });
    await waitFor(() => expect(result.current.task?.id).toBe("task-2"));
    await act(async () => oldTask.resolve(task(TASK_ID)));

    expect(result.current.task?.id).toBe("task-2");
    expect(result.current.status).toBe("success");
  });

  it("rejects a task returned from a different workspace", async () => {
    apiMocks.fetchTask.mockResolvedValueOnce(task(TASK_ID, "other-workspace"));
    const { result } = renderHook(() =>
      useChangeWorkflowTask({ open: true, taskId: TASK_ID, workspaceId: WORKSPACE_ID }),
    );

    await waitFor(() => expect(result.current.status).toBe("error"));
    expect(result.current.task).toBeNull();
    expect(result.current.error).toBeInstanceOf(Error);
  });

  it("discards a refresh response after the task hook is disposed", async () => {
    const refresh = deferred<Task>();
    apiMocks.fetchTask.mockResolvedValueOnce(task(TASK_ID)).mockReturnValueOnce(refresh.promise);
    const { result, unmount } = renderHook(() =>
      useChangeWorkflowTask({ open: true, taskId: TASK_ID, workspaceId: WORKSPACE_ID }),
    );
    await waitFor(() => expect(result.current.status).toBe("success"));
    let pendingRefresh!: Promise<Task | null>;
    act(() => {
      pendingRefresh = result.current.refreshTask();
    });
    unmount();

    await act(async () => refresh.resolve(task(TASK_ID)));
    await expect(pendingRefresh).resolves.toBeNull();
  });

  it("keeps the newest same-task refresh when requests resolve out of order", async () => {
    const firstRefresh = deferred<Task>();
    const secondRefresh = deferred<Task>();
    apiMocks.fetchTask
      .mockResolvedValueOnce(task(TASK_ID))
      .mockReturnValueOnce(firstRefresh.promise)
      .mockReturnValueOnce(secondRefresh.promise);
    const { result } = renderHook(() =>
      useChangeWorkflowTask({ open: true, taskId: TASK_ID, workspaceId: WORKSPACE_ID }),
    );
    await waitFor(() => expect(result.current.status).toBe("success"));

    let firstResult: Promise<Task | null>;
    let secondResult: Promise<Task | null>;
    act(() => {
      firstResult = result.current.refreshTask();
      secondResult = result.current.refreshTask();
    });
    await act(async () => secondRefresh.resolve(task(TASK_ID, WORKSPACE_ID)));
    await expect(secondResult!).resolves.toMatchObject({ id: TASK_ID });
    await act(async () => firstRefresh.resolve({ ...task(TASK_ID), title: "stale" }));

    await expect(firstResult!).resolves.toBeNull();
    expect(result.current.task?.title).not.toBe("stale");
    expect(result.current.status).toBe("success");
  });
});
