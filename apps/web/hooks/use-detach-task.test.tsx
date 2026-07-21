import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Task } from "@/lib/types/http";
import { useDetachTask } from "./use-detach-task";

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
