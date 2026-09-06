import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Canvas } from "@/lib/api/domains/canvas-api";

const listTaskCanvasesMock = vi.hoisted(() => vi.fn());

const TASK_ID = "task-1";
const WORKSPACE_ID = "workspace-1";

vi.mock("@/lib/api/domains/canvas-api", () => ({
  listTaskCanvases: listTaskCanvasesMock,
}));

import { useTaskCanvases } from "./use-task-canvases";

const canvas: Canvas = {
  id: "canvas-1",
  plugin_instance_id: "instance-1",
  plugin_id: "plugin-1",
  workspace_id: WORKSPACE_ID,
  task_id: TASK_ID,
  scope_kind: "task",
  title: "Task canvas",
  status: "active",
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

beforeEach(() => {
  listTaskCanvasesMock.mockReset().mockResolvedValue({ canvases: [canvas] });
});

afterEach(cleanup);

describe("useTaskCanvases", () => {
  it("loads canvases for the task and workspace", async () => {
    const { result } = renderHook(() => useTaskCanvases(TASK_ID, WORKSPACE_ID));

    await waitFor(() => expect(result.current).toEqual([canvas]));
    expect(listTaskCanvasesMock).toHaveBeenCalledWith(TASK_ID, {
      workspaceId: WORKSPACE_ID,
      cache: "no-store",
    });
  });

  it("does not request canvases when disabled", () => {
    const { result } = renderHook(() => useTaskCanvases(TASK_ID, WORKSPACE_ID, false));

    expect(result.current).toEqual([]);
    expect(listTaskCanvasesMock).not.toHaveBeenCalled();
  });

  it("ignores a stale request after the task changes", async () => {
    const first = deferred<{ canvases: Canvas[] }>();
    const second = deferred<{ canvases: Canvas[] }>();
    const nextCanvas = { ...canvas, id: "canvas-2", task_id: "task-2" };
    listTaskCanvasesMock.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);

    const { result, rerender } = renderHook(
      ({ taskId }: { taskId: string }) => useTaskCanvases(taskId, WORKSPACE_ID),
      { initialProps: { taskId: TASK_ID } },
    );

    rerender({ taskId: "task-2" });
    await waitFor(() => expect(listTaskCanvasesMock).toHaveBeenCalledTimes(2));

    second.resolve({ canvases: [nextCanvas] });
    await waitFor(() => expect(result.current).toEqual([nextCanvas]));

    await act(async () => {
      first.resolve({ canvases: [canvas] });
      await first.promise;
    });
    expect(result.current).toEqual([nextCanvas]);
    expect(listTaskCanvasesMock).toHaveBeenNthCalledWith(2, "task-2", {
      workspaceId: WORKSPACE_ID,
      cache: "no-store",
    });
  });
});
