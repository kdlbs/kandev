import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { getTaskGraph, type TaskGraphResponse } from "@/lib/api/domains/task-graph-api";
import { taskGraphSignature, useTaskGraph } from "./use-task-graph";

type StoreTask = {
  id: string;
  title: string;
  state: string;
  parentTaskId?: string | null;
  dependsOn?: Array<{ id: string }>;
  blocks?: Array<{ id: string }>;
  related?: Array<{ id: string }>;
};

const store = vi.hoisted(() => ({
  state: { kanban: { tasks: [] as unknown[] }, kanbanMulti: { snapshots: {} } },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof store.state) => unknown) => selector(store.state),
}));

vi.mock("@/lib/api/domains/task-graph-api", () => ({ getTaskGraph: vi.fn() }));

function graph(mode: "subtasks" | "chain", ids: string[]): TaskGraphResponse {
  return {
    mode,
    available_modes: ["subtasks", "chain"],
    focus_task_id: "p",
    root: mode === "subtasks" ? { id: "p", title: "Parent" } : undefined,
    nodes: ids.map((id) => ({
      id,
      title: id,
      state: "TODO",
      depth: 1,
      role: "member",
      archived: false,
      blocked: false,
    })),
    edges: [],
    truncated: false,
  };
}

function setTasks(tasks: StoreTask[]) {
  store.state = { ...store.state, kanban: { tasks } };
}

beforeEach(() => {
  setTasks([]);
  vi.mocked(getTaskGraph).mockReset();
});

afterEach(() => vi.useRealTimers());

describe("taskGraphSignature", () => {
  it("covers displayed tasks and new children of watched tasks only", () => {
    const watched = new Set(["p", "a"]);
    const base = taskGraphSignature([{ id: "a", title: "A", state: "TODO" }], {}, watched);
    expect(
      taskGraphSignature(
        [
          { id: "a", title: "A", state: "TODO" },
          { id: "x", title: "X", state: "TODO" },
        ],
        {},
        watched,
      ),
    ).toBe(base);
    expect(taskGraphSignature([{ id: "a", title: "A", state: "COMPLETED" }], {}, watched)).not.toBe(
      base,
    );
    expect(
      taskGraphSignature(
        [
          { id: "a", title: "A", state: "TODO" },
          { id: "n", title: "N", state: "TODO", parentTaskId: "p" },
        ],
        {},
        watched,
      ),
    ).not.toBe(base);
    expect(
      taskGraphSignature(
        [{ id: "a", title: "A", state: "TODO", dependsOn: [{ id: "z" } as never] }],
        {},
        watched,
      ),
    ).not.toBe(base);
    expect(
      taskGraphSignature(
        [{ id: "a", title: "A", state: "TODO", related: [{ id: "z" }] }],
        {},
        watched,
      ),
    ).not.toBe(base);
  });
});

describe("useTaskGraph", () => {
  it("loads the default mode, then switches mode on request", async () => {
    vi.mocked(getTaskGraph).mockResolvedValueOnce(graph("subtasks", ["a"]));
    const { result } = renderHook(() => useTaskGraph("p", true));
    expect(result.current.loading).toBe(true);
    await waitFor(() => expect(result.current.model?.mode).toBe("subtasks"));
    expect(getTaskGraph).toHaveBeenCalledWith("p", undefined);

    vi.mocked(getTaskGraph).mockResolvedValueOnce(graph("chain", ["p"]));
    act(() => result.current.setMode("chain"));
    await waitFor(() => expect(result.current.model?.mode).toBe("chain"));
    expect(getTaskGraph).toHaveBeenLastCalledWith("p", "chain");
  });

  it("does not fetch while closed", () => {
    renderHook(() => useTaskGraph("p", false));
    expect(getTaskGraph).not.toHaveBeenCalled();
  });

  it("reports an error and retries", async () => {
    vi.mocked(getTaskGraph).mockRejectedValueOnce(new Error("boom"));
    const { result } = renderHook(() => useTaskGraph("p", true));
    await waitFor(() => expect(result.current.error).toBe(true));
    vi.mocked(getTaskGraph).mockResolvedValueOnce(graph("subtasks", ["a"]));
    act(() => result.current.retry());
    await waitFor(() => expect(result.current.model).not.toBeNull());
    expect(result.current.error).toBe(false);
  });

  it("refetches once, debounced, when a displayed task changes and keeps the old graph meanwhile", async () => {
    setTasks([{ id: "a", title: "A", state: "TODO" }]);
    vi.mocked(getTaskGraph).mockResolvedValue(graph("subtasks", ["a"]));
    const { result, rerender } = renderHook(() => useTaskGraph("p", true));
    await waitFor(() => expect(result.current.model).not.toBeNull());
    expect(getTaskGraph).toHaveBeenCalledTimes(1);

    vi.useFakeTimers();
    setTasks([{ id: "a", title: "A", state: "IN_PROGRESS" }]);
    rerender();
    setTasks([{ id: "a", title: "A", state: "COMPLETED" }]);
    rerender();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(499);
    });
    expect(getTaskGraph).toHaveBeenCalledTimes(1);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(getTaskGraph).toHaveBeenCalledTimes(2);
    expect(getTaskGraph).toHaveBeenLastCalledWith("p", "subtasks");
    expect(result.current.model).not.toBeNull();
  });
});
