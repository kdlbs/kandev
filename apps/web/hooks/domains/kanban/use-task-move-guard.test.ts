import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { Task } from "@/components/kanban-card";
import { useTaskMoveGuard } from "./use-task-move-guard";

function makeTask(id: string): Task {
  return { id, title: id, workflowStepId: "step-2" } as Task;
}

describe("useTaskMoveGuard — the in-flight guard is per task, not list-wide (AC-UI-PIPELINE-ROW-005.2)", () => {
  it("keeps task A guarded while A's move is in flight, unaffected by task B's independent move starting and finishing", async () => {
    const resolvers: Record<string, () => void> = {};
    const moveTask = vi.fn(
      (task: Task) =>
        new Promise<void>((resolve) => {
          resolvers[task.id] = resolve;
        }),
    );
    const { result } = renderHook(() => useTaskMoveGuard(moveTask));
    const taskA = makeTask("task-A");
    const taskB = makeTask("task-B");

    act(() => {
      void result.current.handleMoveTask(taskA, "step-3");
    });
    await waitFor(() => expect(result.current.movingTaskIds.has("task-A")).toBe(true));

    act(() => {
      void result.current.handleMoveTask(taskB, "step-3");
    });
    await waitFor(() => expect(result.current.movingTaskIds.has("task-B")).toBe(true));
    expect(result.current.movingTaskIds.has("task-A")).toBe(true);

    await act(async () => {
      resolvers["task-B"]?.();
      await Promise.resolve();
    });
    await waitFor(() => expect(result.current.movingTaskIds.has("task-B")).toBe(false));
    // Task B settling must not clear task A's still-in-flight guard.
    expect(result.current.movingTaskIds.has("task-A")).toBe(true);

    await act(async () => {
      resolvers["task-A"]?.();
      await Promise.resolve();
    });
    await waitFor(() => expect(result.current.movingTaskIds.has("task-A")).toBe(false));
  });

  it("clears the guard when the move rejects", async () => {
    const moveTask = vi.fn(() => Promise.reject(new Error("move failed")));
    const { result } = renderHook(() => useTaskMoveGuard(moveTask));
    const task = makeTask("task-A");

    await act(async () => {
      await result.current.handleMoveTask(task, "step-3").catch(() => undefined);
    });

    expect(result.current.movingTaskIds.has("task-A")).toBe(false);
  });

  it("ignores a second move request for the same task while the first is still in flight", async () => {
    let resolveFirst: () => void = () => undefined;
    const moveTask = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          resolveFirst = resolve;
        }),
    );
    const { result } = renderHook(() => useTaskMoveGuard(moveTask));
    const task = makeTask("task-A");

    act(() => {
      void result.current.handleMoveTask(task, "step-3");
    });
    await waitFor(() => expect(result.current.movingTaskIds.has("task-A")).toBe(true));

    // A second request for the same task while the first is still in flight
    // must be ignored, not queued: AC-UI-PIPELINE-ROW-005.2.
    await act(async () => {
      await result.current.handleMoveTask(task, "step-4");
    });

    expect(moveTask).toHaveBeenCalledTimes(1);
    expect(moveTask).toHaveBeenCalledWith(task, "step-3");

    await act(async () => {
      resolveFirst();
      await Promise.resolve();
    });
    await waitFor(() => expect(result.current.movingTaskIds.has("task-A")).toBe(false));
  });
});
