import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Task } from "@/lib/types/http";
import { createAppStore } from "@/lib/state/store";
import { useDetachTask, useTaskDetachDialog } from "./use-detach-task";

const detachTaskMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api", () => ({ detachTask: detachTaskMock }));

describe("useDetachTask", () => {
  beforeEach(() => {
    detachTaskMock.mockReset();
  });

  it("reuses the in-flight request for repeated submissions", async () => {
    let resolveRequest!: (task: Task) => void;
    const request = new Promise<Task>((resolve) => {
      resolveRequest = resolve;
    });
    detachTaskMock.mockReturnValueOnce(request);
    const { result } = renderHook(() => useDetachTask());

    let first!: Promise<Task>;
    let second!: Promise<Task>;
    act(() => {
      first = result.current.detachTask("child-1");
      second = result.current.detachTask("child-1");
    });

    expect(second).toBe(first);
    expect(detachTaskMock).toHaveBeenCalledOnce();

    await act(async () => {
      resolveRequest({ id: "child-1" } as Task);
      await first;
    });
    expect(result.current.detachingTaskId).toBeNull();
  });

  it("reuses the in-flight request across hook instances", async () => {
    let resolveRequest!: (task: Task) => void;
    const request = new Promise<Task>((resolve) => {
      resolveRequest = resolve;
    });
    detachTaskMock.mockReturnValueOnce(request);
    const firstHook = renderHook(() => useDetachTask());
    const secondHook = renderHook(() => useDetachTask());

    let first!: Promise<Task>;
    let second!: Promise<Task>;
    act(() => {
      first = firstHook.result.current.detachTask("child-1");
      second = secondHook.result.current.detachTask("child-1");
    });

    expect(second).toBe(first);
    expect(detachTaskMock).toHaveBeenCalledOnce();
    expect(firstHook.result.current.detachingTaskId).toBe("child-1");
    expect(secondHook.result.current.detachingTaskId).toBe("child-1");

    await act(async () => {
      resolveRequest({ id: "child-1" } as Task);
      await first;
    });
    expect(firstHook.result.current.detachingTaskId).toBeNull();
    expect(secondHook.result.current.detachingTaskId).toBeNull();
  });
});

it("confirms detachment from a bounded page without loading a workflow snapshot", () => {
  const store = createAppStore();
  const child = {
    id: "page-child",
    title: "Child",
    parentTaskId: "parent",
    workspaceMode: "inherit_parent" as const,
  };
  const { result } = renderHook(() => useTaskDetachDialog(store, [child]));
  act(() => result.current.handleDetachTask(child.id));
  expect(result.current.detachingTask).toEqual({
    id: child.id,
    title: child.title,
    workspaceMode: "inherit_parent",
  });
  expect(store.getState().kanban.tasks).toEqual([]);
  expect(store.getState().kanbanMulti.snapshots).toEqual({});
});
