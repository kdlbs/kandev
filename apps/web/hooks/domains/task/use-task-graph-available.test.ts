import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const deps = vi.hoisted(() => ({ value: null as unknown }));
const subtasks = vi.hoisted(() => ({ value: [] as unknown[] }));
const count = vi.hoisted(() => ({ calls: [] as Array<[boolean, string | undefined]>, total: 0 }));
const store = vi.hoisted(() => ({
  state: { kanban: { tasks: [] as unknown[] }, kanbanMulti: { snapshots: {} } },
}));

vi.mock("@/hooks/domains/task/use-task-dependencies", () => ({
  useTaskDependencies: () => deps.value,
}));
vi.mock("@/hooks/domains/kanban/use-task-subtasks", () => ({
  useTaskSubtasks: () => subtasks.value,
}));
vi.mock("@/hooks/use-subtask-count", () => ({
  useSubtaskCountState: (open: boolean, taskId?: string) => {
    count.calls.push([open, taskId]);
    return { status: open ? "resolved" : "idle", total: open ? count.total : 0 };
  },
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof store.state) => unknown) => selector(store.state),
}));

import { useTaskGraphAvailable } from "./use-task-graph-available";

beforeEach(() => {
  deps.value = null;
  subtasks.value = [];
  count.calls = [];
  count.total = 0;
  store.state = {
    kanban: { tasks: [{ id: "t", parentTaskId: null }] },
    kanbanMulti: { snapshots: {} },
  };
});

describe("useTaskGraphAvailable", () => {
  it("is false for an isolated task after the server reports no subtasks", () => {
    const { result } = renderHook(() => useTaskGraphAvailable("t"));
    expect(result.current).toBe(false);
    expect(count.calls.at(-1)).toEqual([true, "t"]);
  });

  it("is true with dependency edges, without asking for a subtask count", () => {
    deps.value = { dependsOn: [{ id: "x" }], blocks: [] };
    const { result } = renderHook(() => useTaskGraphAvailable("t"));
    expect(result.current).toBe(true);
    expect(count.calls.every(([open]) => !open)).toBe(true);
  });

  it("is true for a related-only task", () => {
    store.state = {
      kanban: { tasks: [{ id: "t", related: [{ id: "other" }] }] },
      kanbanMulti: { snapshots: {} },
    };
    expect(renderHook(() => useTaskGraphAvailable("t")).result.current).toBe(true);
  });

  it("is true for a subtask", () => {
    store.state = {
      kanban: { tasks: [{ id: "t", parentTaskId: "p" }] },
      kanbanMulti: { snapshots: {} },
    };
    expect(renderHook(() => useTaskGraphAvailable("t")).result.current).toBe(true);
  });

  it("is true with subtasks in the store or on the server", () => {
    subtasks.value = [{ id: "c" }];
    expect(renderHook(() => useTaskGraphAvailable("t")).result.current).toBe(true);
    subtasks.value = [];
    count.total = 2;
    expect(renderHook(() => useTaskGraphAvailable("t")).result.current).toBe(true);
  });

  it("is false without a task id", () => {
    expect(renderHook(() => useTaskGraphAvailable(null)).result.current).toBe(false);
  });
});
