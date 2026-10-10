import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Task } from "@/lib/types/http";

const fetchTaskMock = vi.hoisted(() => vi.fn());
const TASK_WORKSPACE = "task-workspace";
const LOOKUP_TASK_ID = "task-1";
const TASK_SESSION_ID = "task-session";
const TEMPORARY_FAILURE = "temporary failure";
const CACHED_WORKSPACE = "cached-workspace";
const CONFLICTING_WORKSPACE = "conflicting-workspace";
const RETRY_WORKSPACE = "retry-workspace";
const mockState = {
  quickChat: { sessions: [] },
  kanban: { workflowId: null, tasks: [] },
  kanbanMulti: { snapshots: {} },
  workflows: { items: [] },
  office: { tasks: { items: [] as Array<{ id: string; workspaceId: string }> } },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mockState) => unknown) => selector(mockState),
}));

vi.mock("@/lib/api/domains/kanban-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/kanban-api")>()),
  fetchTask: fetchTaskMock,
}));

import { useComposerWorkspace } from "./use-composer-workspace";

function task(workspaceId: string): Task {
  return { workspace_id: workspaceId } as Task;
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

beforeEach(() => {
  fetchTaskMock.mockReset();
  mockState.office.tasks.items = [];
});
afterEach(cleanup);

// eslint-disable-next-line max-lines-per-function -- scenarios share the lookup lifecycle contract.
describe("useComposerWorkspace", () => {
  it("does not look up a workspace when there is no task", () => {
    const { result } = renderHook(() => useComposerWorkspace(null, null));

    expect(result.current).toMatchObject({ workspaceId: null, status: "resolved" });
    expect(fetchTaskMock).not.toHaveBeenCalled();
  });

  it("fetches the authoritative workspace for a cold task route", async () => {
    fetchTaskMock.mockResolvedValue(task(TASK_WORKSPACE));
    const { result } = renderHook(() => useComposerWorkspace(TASK_SESSION_ID, LOOKUP_TASK_ID));

    await waitFor(() => expect(result.current.workspaceId).toBe(TASK_WORKSPACE));
    expect(fetchTaskMock).toHaveBeenCalledWith(LOOKUP_TASK_ID, { cache: "no-store" });
  });

  it("deduplicates a cold task lookup across mounted composers", async () => {
    const request = deferred<Task>();
    fetchTaskMock.mockReturnValue(request.promise);
    const first = renderHook(() => useComposerWorkspace("session-1", LOOKUP_TASK_ID));
    const second = renderHook(() => useComposerWorkspace("session-2", LOOKUP_TASK_ID));

    await waitFor(() => expect(fetchTaskMock).toHaveBeenCalledTimes(1));
    await act(async () => request.resolve(task(TASK_WORKSPACE)));

    await waitFor(() => expect(first.result.current.workspaceId).toBe(TASK_WORKSPACE));
    expect(second.result.current.workspaceId).toBe(TASK_WORKSPACE);
  });

  it("can retry after the task lookup fails", async () => {
    fetchTaskMock
      .mockRejectedValueOnce(new Error(TEMPORARY_FAILURE))
      .mockResolvedValueOnce(task(TASK_WORKSPACE));
    const { result } = renderHook(() => useComposerWorkspace(TASK_SESSION_ID, LOOKUP_TASK_ID));

    await waitFor(() => expect(result.current.status).toBe("failed"));
    act(() => result.current.retry());
    await waitFor(() => expect(result.current.workspaceId).toBe(TASK_WORKSPACE));
    expect(fetchTaskMock).toHaveBeenCalledTimes(2);
  });

  it("ignores a late task lookup after the composer switches task identity", async () => {
    const oldRequest = deferred<Task>();
    const newRequest = deferred<Task>();
    fetchTaskMock.mockReturnValueOnce(oldRequest.promise).mockReturnValueOnce(newRequest.promise);
    const { result, rerender } = renderHook(
      ({ taskId }: { taskId: string }) => useComposerWorkspace(null, taskId),
      { initialProps: { taskId: LOOKUP_TASK_ID } },
    );

    rerender({ taskId: "task-2" });
    await waitFor(() => expect(fetchTaskMock).toHaveBeenCalledTimes(2));
    await act(async () => newRequest.resolve(task("workspace-2")));
    await waitFor(() => expect(result.current.workspaceId).toBe("workspace-2"));
    await act(async () => oldRequest.resolve(task("workspace-1")));

    expect(result.current.workspaceId).toBe("workspace-2");
  });

  it.each([
    { cacheTransition: "disappears", outcome: "resolve" },
    { cacheTransition: "becomes conflicting", outcome: "reject" },
  ] as const)(
    "reattaches to a pending lookup when cached scope $cacheTransition",
    async ({ cacheTransition, outcome }) => {
      const taskId = `cache-${cacheTransition}`;
      const request = deferred<Task>();
      fetchTaskMock
        .mockReturnValueOnce(request.promise)
        .mockResolvedValueOnce(task(RETRY_WORKSPACE));
      const { result, rerender } = renderHook(() => useComposerWorkspace(TASK_SESSION_ID, taskId));

      await waitFor(() => expect(fetchTaskMock).toHaveBeenCalledTimes(1));
      act(() => {
        mockState.office.tasks.items = [{ id: taskId, workspaceId: CACHED_WORKSPACE }];
        rerender();
      });
      expect(result.current).toMatchObject({ workspaceId: null, status: "loading" });

      act(() => {
        mockState.office.tasks.items =
          cacheTransition === "disappears"
            ? []
            : [
                { id: taskId, workspaceId: CACHED_WORKSPACE },
                { id: taskId, workspaceId: CONFLICTING_WORKSPACE },
              ];
        rerender();
      });

      if (outcome === "resolve") {
        await act(async () => request.resolve(task(TASK_WORKSPACE)));
        await waitFor(() => expect(result.current.workspaceId).toBe(TASK_WORKSPACE));
      } else {
        await act(async () => request.reject(new Error(TEMPORARY_FAILURE)));
        await waitFor(() => expect(result.current.status).toBe("failed"));
        act(() => result.current.retry());
        await waitFor(() => expect(result.current.workspaceId).toBe(RETRY_WORKSPACE));
      }

      expect(fetchTaskMock).toHaveBeenCalledTimes(outcome === "resolve" ? 1 : 2);
    },
  );

  it("keeps authoritative failure and retry state while matching cache is present", async () => {
    const taskId = "authoritative-failure";
    const request = deferred<Task>();
    fetchTaskMock.mockReturnValueOnce(request.promise).mockResolvedValueOnce(task(RETRY_WORKSPACE));
    const { result, rerender } = renderHook(() => useComposerWorkspace(TASK_SESSION_ID, taskId));

    await waitFor(() => expect(fetchTaskMock).toHaveBeenCalledTimes(1));
    act(() => {
      mockState.office.tasks.items = [{ id: taskId, workspaceId: CACHED_WORKSPACE }];
      rerender();
    });
    expect(result.current).toMatchObject({ workspaceId: null, status: "loading" });

    await act(async () => request.reject(new Error(TEMPORARY_FAILURE)));
    await waitFor(() =>
      expect(result.current).toMatchObject({ workspaceId: null, status: "failed" }),
    );
    act(() => result.current.retry());
    await waitFor(() => expect(result.current.workspaceId).toBe(RETRY_WORKSPACE));
    expect(fetchTaskMock).toHaveBeenCalledTimes(2);
  });

  it.each(["resolve", "reject"] as const)(
    "reattaches to a pending lookup when task identity changes A -> null -> A and the old request %s",
    async (outcome) => {
      const request = deferred<Task>();
      fetchTaskMock
        .mockReturnValueOnce(request.promise)
        .mockResolvedValueOnce(task(RETRY_WORKSPACE));
      const { result, rerender } = renderHook(
        ({ taskId }: { taskId: string | null }) => useComposerWorkspace(null, taskId),
        { initialProps: { taskId: "task-a" as string | null } },
      );

      await waitFor(() => expect(fetchTaskMock).toHaveBeenCalledTimes(1));
      rerender({ taskId: null });
      rerender({ taskId: "task-a" });

      if (outcome === "resolve") {
        await act(async () => request.resolve(task("workspace-a")));
        await waitFor(() => expect(result.current.workspaceId).toBe("workspace-a"));
        expect(fetchTaskMock).toHaveBeenCalledTimes(1);
      } else {
        await act(async () => request.reject(new Error(TEMPORARY_FAILURE)));
        await waitFor(() => expect(result.current.status).toBe("failed"));
        act(() => result.current.retry());
        await waitFor(() => expect(result.current.workspaceId).toBe(RETRY_WORKSPACE));
        expect(fetchTaskMock).toHaveBeenCalledTimes(2);
      }
    },
  );
});
