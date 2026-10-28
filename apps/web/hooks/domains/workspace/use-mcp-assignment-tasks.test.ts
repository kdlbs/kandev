import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { listTasksByWorkspace } from "@/lib/api/domains/kanban-api";
import type { ListTasksResponse, Task } from "@/lib/types/http";
import { taskId, workflowId, workspaceId } from "@/lib/types/ids";
import { useMCPAssignmentTasks } from "./use-mcp-assignment-tasks";

vi.mock("@/lib/api/domains/kanban-api", () => ({ listTasksByWorkspace: vi.fn() }));

function task(id: string, overrides: Partial<Task> = {}): Task {
  return {
    id: taskId(id),
    workspace_id: workspaceId("ws"),
    workflow_id: workflowId("flow"),
    workflow_step_id: "step",
    position: 0,
    title: id,
    description: "",
    state: "TODO",
    priority: "medium",
    created_at: "2026-10-25T00:00:00Z",
    updated_at: "2026-10-25T00:00:00Z",
    ...overrides,
  };
}
function deferred() {
  let resolve!: (value: ListTasksResponse) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<ListTasksResponse>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(listTasksByWorkspace).mockResolvedValue({ tasks: [], total: 0 });
});

// @covers AC-AGENTS-MCP-ASSIGNMENT-002.4, AC-AGENTS-MCP-ASSIGNMENT-003.4
it("loads only the requested page when the task chooser is active", async () => {
  vi.mocked(listTasksByWorkspace).mockResolvedValue({ tasks: [task("match")], total: 51 });
  const { result, rerender } = renderHook(
    ({ active, page }) => useMCPAssignmentTasks("ws", { active, page, query: "docs" }),
    { initialProps: { active: false, page: 1 } },
  );
  expect(listTasksByWorkspace).not.toHaveBeenCalled();
  rerender({ active: true, page: 2 });
  await waitFor(() => expect(result.current.tasks).toEqual([task("match")]));
  expect(result.current.total).toBe(51);
  expect(listTasksByWorkspace).toHaveBeenCalledTimes(1);
  expect(listTasksByWorkspace).toHaveBeenCalledWith(
    "ws",
    { page: 2, pageSize: 25, query: "docs", includeArchived: false },
    { init: { signal: expect.any(AbortSignal) } },
  );
});

it("does not offer archived, orphaned, or foreign-workspace task rows", async () => {
  vi.mocked(listTasksByWorkspace).mockResolvedValue({
    tasks: [
      task("valid"),
      task("archived", { archived_at: "2026-10-25T00:00:00Z" }),
      task("orphan", { workspace_orphaned: true }),
      task("foreign", { workspace_id: workspaceId("other") }),
    ],
    total: 4,
  });
  const { result } = renderHook(() =>
    useMCPAssignmentTasks("ws", { active: true, page: 1, query: "" }),
  );
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.tasks.map((row) => row.id)).toEqual(["valid"]);
  expect(result.current.total).toBe(4);
});

it.each(["resolve", "reject"] as const)(
  "ignores old %s after leaving and returning to a query",
  async (outcome) => {
    const old = deferred();
    vi.mocked(listTasksByWorkspace).mockReturnValueOnce(old.promise);
    const { result, rerender } = renderHook(
      ({ workspace, query }) => useMCPAssignmentTasks(workspace, { active: true, page: 1, query }),
      { initialProps: { workspace: "ws", query: "first" } },
    );
    rerender({ workspace: "other", query: "second" });
    vi.mocked(listTasksByWorkspace).mockResolvedValueOnce({ tasks: [task("current")], total: 1 });
    rerender({ workspace: "ws", query: "first" });
    await waitFor(() => expect(result.current.tasks.map((row) => row.id)).toEqual(["current"]));
    await act(async () => {
      if (outcome === "resolve") old.resolve({ tasks: [task("stale")], total: 99 });
      else old.reject(new Error("private stale error"));
    });
    expect(result.current.tasks.map((row) => row.id)).toEqual(["current"]);
    expect(result.current.failed).toBe(false);
    expect(result.current.loading).toBe(false);
  },
);

it("keeps errors generic and retries the read without retaining stale rows", async () => {
  vi.mocked(listTasksByWorkspace).mockRejectedValueOnce(new Error("private details"));
  const { result } = renderHook(() =>
    useMCPAssignmentTasks("ws", { active: true, page: 1, query: "" }),
  );
  await waitFor(() => expect(result.current.failed).toBe(true));
  expect(result.current.tasks).toEqual([]);
  expect(result.current.loading).toBe(false);
  vi.mocked(listTasksByWorkspace).mockResolvedValueOnce({ tasks: [task("recovered")], total: 1 });
  act(() => result.current.reload());
  await waitFor(() => expect(result.current.tasks.map((row) => row.id)).toEqual(["recovered"]));
  expect(result.current.failed).toBe(false);
});

it("cancels pending discovery on close and starts a fresh read on reopen", async () => {
  const old = deferred();
  vi.mocked(listTasksByWorkspace).mockReturnValueOnce(old.promise);
  const { result, rerender, unmount } = renderHook(
    ({ active }) => useMCPAssignmentTasks("ws", { active, page: 1, query: "" }),
    { initialProps: { active: true } },
  );
  const signal = vi.mocked(listTasksByWorkspace).mock.calls[0][2]?.init?.signal;
  rerender({ active: false });
  expect(signal?.aborted).toBe(true);
  await act(async () => old.resolve({ tasks: [task("closed")], total: 1 }));
  expect(result.current.tasks).toEqual([]);
  expect(result.current.loading).toBe(false);
  const next = deferred();
  vi.mocked(listTasksByWorkspace).mockReturnValueOnce(next.promise);
  rerender({ active: true });
  expect(result.current.loading).toBe(true);
  const nextSignal = vi.mocked(listTasksByWorkspace).mock.calls[1][2]?.init?.signal;
  unmount();
  expect(nextSignal?.aborted).toBe(true);
  await act(async () => next.resolve({ tasks: [task("unmounted")], total: 1 }));
});
