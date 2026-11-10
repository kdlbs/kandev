import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { Task } from "@/lib/types/http";
import { useTasksListData } from "@/hooks/use-tasks-list-data";

const list = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api", () => ({ listTasksByWorkspace: list }));
const query = {
  archiveMode: "active" as const,
  searchQuery: "",
  workflowId: null,
  repositoryId: null,
};
const base = {
  workspaceId: "ws",
  query,
  sort: "updated_desc",
  pagination: { pageIndex: 0, pageSize: 25 },
  enabled: true,
  onPageChange: vi.fn(),
};
function response(id: string, total = 1) {
  return { tasks: [{ id, title: id, workspace_id: "ws" } as Task], total };
}
function deferred() {
  let resolve!: (value: ReturnType<typeof response>) => void;
  const promise = new Promise<ReturnType<typeof response>>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}
beforeEach(() => {
  list.mockReset();
  base.onPageChange.mockClear();
});
afterEach(cleanup);

it("clears rows and cancels a read when the workspace disappears", async () => {
  const pending = deferred();
  list.mockReturnValue(pending.promise);
  const { result, rerender } = renderHook((props) => useTasksListData(props), {
    initialProps: { ...base, workspaceId: "ws" as string | null },
  });
  const signal = list.mock.calls[0][2].init.signal as AbortSignal;
  rerender({ ...base, workspaceId: null });
  expect(result.current).toMatchObject({ tasks: [], total: 0, isLoading: false });
  expect(signal.aborted).toBe(true);
  await act(async () => pending.resolve(response("old")));
  expect(result.current.tasks).toEqual([]);
});

// @covers AC-TASKS-LIST-ARCHIVE-FILTERS-004.2
it("rejects a delayed response after a different query wins in the same workspace", async () => {
  const old = deferred();
  const current = deferred();
  list.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise);
  const { result, rerender } = renderHook((props) => useTasksListData(props), {
    initialProps: base,
  });
  rerender({ ...base, query: { ...query, searchQuery: "current" } });
  await act(async () => current.resolve(response("current")));
  await act(async () => old.resolve(response("old")));
  expect(result.current.tasks.map((task) => task.id)).toEqual(["current"]);
});

it("hides previous rows while replacement intent waits for its debounce", async () => {
  list.mockResolvedValue(response("old"));
  const { result, rerender } = renderHook((props) => useTasksListData(props), {
    initialProps: base,
  });
  await waitFor(() => expect(result.current.tasks).toHaveLength(1));
  rerender({ ...base, query: { ...query, searchQuery: "next" }, enabled: false });
  expect(result.current.tasks).toEqual([]);
  expect(result.current.isLoading).toBe(true);
  expect(result.current.total).toBe(0);
});

// @covers AC-TASKS-LIST-ARCHIVE-FILTERS-004.4
it("refreshes the latest query even when a mutation retained an earlier refresh callback", async () => {
  list.mockResolvedValue(response("task"));
  const { result, rerender } = renderHook((props) => useTasksListData(props), {
    initialProps: base,
  });
  await waitFor(() => expect(result.current.isLoading).toBe(false));
  const refresh = result.current.fetchTasks;
  rerender({ ...base, query: { ...query, searchQuery: "latest" } });
  await act(async () => refresh());
  expect(list.mock.calls.at(-1)?.[1]).toMatchObject({ query: "latest", page: 1 });
});

it("settles a failed current read and retries its current scope", async () => {
  list.mockRejectedValueOnce(new Error("offline")).mockResolvedValueOnce(response("retry"));
  const { result } = renderHook(() => useTasksListData(base));
  await waitFor(() => expect(result.current.error?.message).toBe("offline"));
  expect(result.current.isLoading).toBe(false);
  await act(async () => result.current.fetchTasks());
  expect(result.current.error).toBeNull();
  expect(result.current.tasks[0]?.id).toBe("retry");
});

it("recovers the last available page after its final task disappears", async () => {
  list.mockResolvedValue({ tasks: [], total: 26 });
  const { result } = renderHook(() =>
    useTasksListData({ ...base, pagination: { pageIndex: 2, pageSize: 25 } }),
  );
  await waitFor(() => expect(base.onPageChange).toHaveBeenCalledWith(1));
  expect(result.current.tasks).toEqual([]);
});

it("uses the built-in API sort while retaining a plugin facet preference", async () => {
  list.mockResolvedValue(response("task"));
  const { result } = renderHook(() => useTasksListData({ ...base, sort: "facet:plugin:tags" }));
  await waitFor(() => expect(result.current.isLoading).toBe(false));
  expect(list.mock.calls.at(-1)?.[1]).toMatchObject({ sort: "updated_desc" });
});
